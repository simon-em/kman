# Homebrew formula for kman. This repository is its own tap, same pattern as
# kranq's Formula/kranq.rb:
#
#   brew tap simon-em/kman https://github.com/simon-em/kman
#   brew install simon-em/kman/kman
#
# No tag has been cut yet: url/sha256 below are placeholders. To cut a
# release: tag it, push the tag, then put the tarball's sha256 here and push
# that. The formula on main always points at the last tag, never at main.
class Kman < Formula
  desc "Manages Slack/human-triggered access to kranq"
  homepage "https://github.com/simon-em/kman"
  # TODO: no tag exists yet.
  url "https://github.com/simon-em/kman/archive/refs/tags/v0.1.0.tar.gz"
  sha256 ""

  head "https://github.com/simon-em/kman.git", branch: "main"

  depends_on "go" => :build
  depends_on :macos

  def install
    ldflags = %W[
      -X github.com/simon-em/kman/internal/cli.Version=#{version}
    ]
    system "go", "build", *std_go_args(ldflags: ldflags)
  end

  def caveats
    <<~EOS
      This is Phase 0: brew installed the binary, and kman can validate and
      render flows. Nothing pushes to kranq yet.
    EOS
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/kman version")

    (testpath/"flow.yaml").write <<~YAML
      name: brew-test
      steps:
        - name: it compiles
          run: echo hello
    YAML
    system bin/"kman", "validate", testpath/"flow.yaml"
    assert_match "echo hello", shell_output("#{bin}/kman render #{testpath}/flow.yaml")
  end
end
