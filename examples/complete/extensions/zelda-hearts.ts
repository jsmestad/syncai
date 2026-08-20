import type { ExtensionAPI } from "@earendil-works/pi-coding-agent"

const MAX_HEARTS = 5
const STATUS_KEY = "zelda-hearts"
const PHRASES = ["you are absolutely right", "you're absolutely right", "great point", "excellent point", "you nailed it"]

function assistantText(message: { role: string; content: string | Array<{ type: string; text?: string }> }): string {
  if (message.role !== "assistant") return ""
  if (typeof message.content === "string") return message.content
  return message.content.filter((part) => part.type === "text" && typeof part.text === "string").map((part) => part.text).join(" ")
}

export default function zeldaHearts(pi: ExtensionAPI) {
  let hearts = MAX_HEARTS

  const render = (ctx: { ui: { setStatus(key: string, value: string | undefined): void } }) => {
    const full = "♥".repeat(hearts)
    const empty = "♡".repeat(MAX_HEARTS - hearts)
    ctx.ui.setStatus(STATUS_KEY, `${full}${empty}`)
  }

  pi.on("session_start", async (_event, ctx) => render(ctx))
  pi.on("message_end", async (event, ctx) => {
    const text = assistantText(event.message).toLowerCase()
    if (!text) return
    const hit = PHRASES.some((phrase) => text.includes(phrase))
    hearts = Math.max(0, Math.min(MAX_HEARTS, hearts + (hit ? -1 : 1)))
    render(ctx)
  })

  pi.registerCommand("revive", {
    description: "Restore the sycophancy meter",
    handler: async (_args, ctx) => {
      hearts = MAX_HEARTS
      render(ctx)
      ctx.ui.notify("Hearts restored", "info")
    },
  })
}
