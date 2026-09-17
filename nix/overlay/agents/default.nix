# Platform-owned agent overlay. docs/DECISIONS.md R3-19: agents come from
# here, not nixpkgs, so the platform decides when a tenant's agent changes.
# Each attribute repackages upstream's binary release; bump.sh updates hashes.
final: prev: {
  # Placeholders resolve to nixpkgs until each package is repackaged here.
  factoryAgents = {
    claude-code = prev.claude-code;
    opencode = prev.opencode;
    codex = prev.codex;
    gemini-cli = prev.gemini-cli;
    pi-coding-agent = prev.pi-coding-agent;
  };
}
