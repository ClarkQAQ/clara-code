package provider

import (
	"fmt"

	"clara-code/pkg/model"
)

func validatePromptRoles(prompt model.Prompt) error {
	for index, message := range prompt {
		switch message.Role {
		case model.MessageRoleSystem, model.MessageRoleDeveloper, model.MessageRoleUser, model.MessageRoleAssistant, model.MessageRoleTool:
		default:
			return fmt.Errorf("message %d: unsupported role %q", index, message.Role)
		}
		if message.Role == model.MessageRoleDeveloper {
			for _, part := range message.Content {
				if _, ok := model.AsMessagePart[model.TextPart](part); !ok {
					return fmt.Errorf("message %d: developer instructions require text content", index)
				}
			}
		}
	}
	return nil
}
