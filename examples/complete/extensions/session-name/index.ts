import type { ExtensionAPI } from "@earendil-works/pi-coding-agent"

export default function sessionName(pi: ExtensionAPI) {
  pi.registerCommand("session-name", {
    description: "Set or show the current session name",
    handler: async (args, ctx) => {
      const name = args.trim()
      if (name) {
        pi.setSessionName(name)
        ctx.ui.notify(`Session named: ${name}`, "info")
        return
      }

      const current = pi.getSessionName()
      ctx.ui.notify(current ? `Session: ${current}` : "No session name set", "info")
    },
  })
}
