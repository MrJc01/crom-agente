# Homebrew formula for crom-agente
class CromAgente < Formula
  desc "Agente autônomo de engenharia de software baseado em LLMs"
  homepage "https://github.com/crom/crom-agente"
  url "https://github.com/crom/crom-agente/archive/refs/tags/v#{version}.tar.gz"
  license "MIT"

  depends_on "go" => :build

  def install
    system "go", "build", *std_go_args(ldflags: "-s -w -X github.com/crom/crom-agente/internal/cli.Version=#{version}"), "./cmd/crom-agente"
    
    # Install shell completions
    generate_completions_from_executable(bin/"crom-agente", "completion")
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/crom-agente --version")
  end
end
