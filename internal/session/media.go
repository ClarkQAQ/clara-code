package session

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"strings"

	"clara-code/pkg/model"
)

const blobPrefix = "clara-blob:"

type Attachment struct {
	Hash                string `json:"hash"`
	Filename            string `json:"filename"`
	MediaType           string `json:"media_type"`
	Width, Height, Size int
}

func SaveAttachment(home string, a Attachment, data []byte) (Attachment, error) {
	hash, err := putMedia(home, data)
	if err != nil {
		return Attachment{}, err
	}
	a.Hash = hash
	if cfg, _, err := image.DecodeConfig(bytes.NewReader(data)); err == nil {
		a.Width = cfg.Width
		a.Height = cfg.Height
	}
	a.Size = len(data)
	a.Filename = filepath.Base(a.Filename)
	if a.Filename == "." || a.Filename == string(filepath.Separator) {
		a.Filename = "image"
	}
	return a, nil
}
func (a Attachment) File(home string) (model.FilePart, error) {
	data, err := getMedia(home, a.Hash)
	if err != nil {
		return model.FilePart{}, err
	}
	return model.FilePart{Filename: a.Filename, MediaType: a.MediaType, Data: data}, nil
}
func putMedia(home string, data []byte) (string, error) {
	if len(data) == 0 || len(data) > 16<<20 {
		return "", errors.New("attachment must contain between 1 byte and 16 MiB")
	}
	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])
	dir := filepath.Join(home, "attachments")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, hash)
	if _, err := os.Lstat(path); err == nil {
		if _, err := getMedia(home, hash); err != nil {
			return "", err
		}
		return hash, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	file, err := os.CreateTemp(dir, ".image-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(file.Name())
	if _, err := file.Write(data); err != nil {
		file.Close()
		return "", err
	}
	if err := errors.Join(file.Sync(), file.Close()); err != nil {
		return "", err
	}
	return hash, os.Rename(file.Name(), path)
}
func getMedia(home, hash string) ([]byte, error) {
	if len(hash) != 64 {
		return nil, errors.New("invalid attachment reference")
	}
	if _, err := hex.DecodeString(hash); err != nil {
		return nil, errors.New("invalid attachment reference")
	}
	path := filepath.Join(home, "attachments", hash)
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 16<<20 {
		return nil, fmt.Errorf("attachment %s is missing or invalid", hash)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != hash {
		return nil, fmt.Errorf("attachment %s is corrupted", hash)
	}
	return data, nil
}

// The disk codec replaces image payloads only; Core and provider types stay unchanged.
func mediaJSON(home string, body []byte, restore bool) ([]byte, error) {
	var tree any
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&tree); err != nil {
		return nil, err
	}
	var walk func(any) error
	walk = func(node any) error {
		switch v := node.(type) {
		case []any:
			for _, item := range v {
				if err := walk(item); err != nil {
					return err
				}
			}
		case map[string]any:
			mime, _ := v["media_type"].(string)
			raw, ok := v["data"].(string)
			if ok && strings.HasPrefix(mime, "image/") && raw != "" {
				if restore && strings.HasPrefix(raw, blobPrefix) {
					data, err := getMedia(home, strings.TrimPrefix(raw, blobPrefix))
					if err != nil {
						return err
					}
					v["data"] = base64.StdEncoding.EncodeToString(data)
				} else if !restore && !strings.HasPrefix(raw, blobPrefix) {
					data, err := base64.StdEncoding.DecodeString(raw)
					if err != nil {
						return errors.New("invalid image encoding in session")
					}
					hash, err := putMedia(home, data)
					if err != nil {
						return err
					}
					v["data"] = blobPrefix + hash
				}
			}
			for _, item := range v {
				if err := walk(item); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := walk(tree); err != nil {
		return nil, err
	}
	return json.Marshal(tree)
}
func SummaryMessages(messages []model.Message) []model.Message {
	out := make([]model.Message, len(messages))
	for i, m := range messages {
		out[i] = m
		out[i].Content = make([]model.MessagePart, 0, len(m.Content))
		for _, part := range m.Content {
			if file, ok := model.AsMessagePart[model.FilePart](part); ok && strings.HasPrefix(file.MediaType, "image/") {
				out[i].Content = append(out[i].Content, model.TextPart{Text: "[Image: " + file.Filename + "]"})
				continue
			}
			if result, ok := model.AsMessagePart[model.ToolResultPart](part); ok {
				if media, ok := model.AsToolResultOutputType[model.ToolResultOutputContentMedia](result.Output); ok && strings.HasPrefix(media.MediaType, "image/") {
					result.Output = model.ToolResultOutputContentText{Text: "[Image tool output] " + media.Text}
					part = result
				}
			}
			out[i].Content = append(out[i].Content, part)
		}
	}
	return out
}
func MediaBytes(messages []model.Message) int {
	n := 0
	for _, m := range messages {
		for _, part := range m.Content {
			if file, ok := model.AsMessagePart[model.FilePart](part); ok {
				n += len(file.Data)
			}
			if result, ok := model.AsMessagePart[model.ToolResultPart](part); ok {
				if media, ok := model.AsToolResultOutputType[model.ToolResultOutputContentMedia](result.Output); ok {
					n += len(media.Data)
				}
			}
		}
	}
	return n
}

func (s *Store) SetSelection(provider, name, agentEffort, planEffort string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.data
	next.Meta.Provider = provider
	next.Meta.Model = name
	next.Meta.Effort = agentEffort
	next.Meta.PlanEffort = planEffort
	if provider != s.data.Meta.Provider || name != s.data.Meta.Model {
		next.Meta.LastUsage = nil
	}
	return s.commit(next)
}
func (s *Store) SetPendingSelection(provider, plan string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.Meta.Provider = provider
	s.data.Meta.PlanEffort = plan
}

func attachmentsFrom(home string, message model.Message) ([]Attachment, error) {
	var out []Attachment
	for _, part := range message.Content {
		if file, ok := model.AsMessagePart[model.FilePart](part); ok && strings.HasPrefix(file.MediaType, "image/") {
			a, err := SaveAttachment(home, Attachment{Filename: file.Filename, MediaType: file.MediaType}, file.Data)
			if err != nil {
				return nil, err
			}
			out = append(out, a)
		}
	}
	return out, nil
}

func (s *Store) SetInitialModel(name, effort string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data.Meta.ID == "" {
		s.data.Meta.Model = name
		s.data.Meta.Effort = effort
	}
}
