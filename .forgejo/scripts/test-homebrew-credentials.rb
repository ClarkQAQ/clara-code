#!/usr/bin/env ruby

# frozen_string_literal: true

require "tmpdir"
require "uri"

class Formula
  def self.desc(*); end
  def self.homepage(*); end
  def self.url(*, **); end
  def self.version(*); end
  def self.sha256(*); end
  def self.license(*); end
  def self.depends_on(*); end
  def self.test(&); end
end

template = File.expand_path("../packaging/homebrew/clara-code.rb.template", __dir__)
load template

def assert_equal(expected, actual, message)
  return if expected == actual

  raise "#{message}: expected #{expected.inspect}, got #{actual.inspect}"
end

def assert_nil(actual, message)
  raise "#{message}: expected nil, got #{actual.inspect}" unless actual.nil?
end

def basic_credentials(header)
  prefix = "Authorization: Basic "
  raise "unexpected authorization header" unless header&.start_with?(prefix)

  header.delete_prefix(prefix).unpack1("m0").split(":", 2)
end

def with_home(path)
  previous_home = ENV["HOME"]
  ENV["HOME"] = path
  yield
ensure
  ENV["HOME"] = previous_home
end

Dir.mktmpdir("clara-code-homebrew-credentials") do |home|
  with_home(home) do
    ENV["HOMEBREW_CLARA_CODE_TOKEN"] = "ignored-token"
    assert_nil(ClaraCode.authorization_header, "environment tokens must be ignored")
    ENV.delete("HOMEBREW_CLARA_CODE_TOKEN")

    File.write(File.join(home, ".git-credentials"), "")
    File.write(File.join(home, ".netrc"), "")
    assert_nil(ClaraCode.authorization_header, "empty credential files must not authenticate")

    File.write(
      File.join(home, ".git-credentials"),
      <<~CREDENTIALS,
        # comment
        not a credential
        https://ignored:wrong@example.com
        https://alice%40example.com:p%3A%2F%2B%20@#{ClaraCode::FORGEJO_HOST}
      CREDENTIALS
    )
    assert_equal(
      ["alice@example.com", "p:/+ "],
      basic_credentials(ClaraCode.authorization_header),
      "git credentials must be decoded and ignore unrelated records",
    )

    File.write(
      File.join(home, ".netrc"),
      <<~NETRC,
        machine example.com
          login ignored
          password ignored
        machine #{ClaraCode::FORGEJO_HOST}
          login "netrc user"
          password "quoted\\\"password"
      NETRC
    )
    assert_equal(
      ["alice@example.com", "p:/+ "],
      basic_credentials(ClaraCode.authorization_header),
      "git credentials must take precedence over netrc",
    )

    File.write(
      File.join(home, ".git-credentials"),
      <<~CREDENTIALS,
        https://malformed%ZZ:password@#{ClaraCode::FORGEJO_HOST}
        https://missing-password@#{ClaraCode::FORGEJO_HOST}
      CREDENTIALS
    )
    assert_equal(
      ["netrc user", "quoted\"password"],
      basic_credentials(ClaraCode.authorization_header),
      "valid netrc credentials must be used when git records are malformed",
    )

    File.delete(File.join(home, ".git-credentials"))
    assert_equal(
      ["netrc user", "quoted\"password"],
      basic_credentials(ClaraCode.authorization_header),
      "netrc credentials must work without git credentials",
    )

    File.write(File.join(home, ".netrc"), "machine example.com login ignored password ignored\n")
    assert_nil(ClaraCode.authorization_header, "missing Forgejo credentials must not authenticate")
  end
end

puts "Homebrew credential tests passed"
