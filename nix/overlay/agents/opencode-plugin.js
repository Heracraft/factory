// repose opencode plugin: relays session events to repose-hook, which POSTs
// them to /run/repose/hooks.sock (docs/interfaces/guest-conventions.md).
// Installed to ~/.config/opencode/plugins/repose.js by repose-agent-setup;
// never overwritten once present, so a user may edit it.
export const ReposeHooks = async ({ $ }) => {
  const send = async (kind, summary) => {
    const body = JSON.stringify({
      agent: "opencode",
      kind,
      summary: String(summary || "").slice(0, 1000),
    });
    try {
      await $`repose-hook ${body}`.quiet().nothrow();
    } catch (_) {
      // a hook failure never blocks the agent
    }
  };
  return {
    event: async ({ event }) => {
      const type = (event && event.type) || "";
      if (type === "session.idle") {
        await send("completed", "opencode finished");
      } else if (type === "session.error") {
        const err = event.properties && event.properties.error;
        await send("error", (err && (err.message || err.name)) || "opencode error");
      } else if (type === "permission.updated" || type === "permission.asked") {
        const p = event.properties || {};
        await send("needs_input", p.title || p.type || "opencode needs permission");
      }
    },
  };
};
