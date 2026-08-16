import z from "zod"

const tool = Object.assign(
  (definition: { description: string; args: Record<string, z.ZodType>; execute: Function }) => definition,
  { schema: z },
)

const endpoint = process.env.TOOL_BRIDGE_URL ?? "http://tool:4096"

async function execute(toolID: string, args: unknown, signal: AbortSignal) {
  const response = await fetch(`${endpoint}/execute`, {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({ tool: toolID, args }),
    signal,
  })
  if (!response.ok) {
    const detail = await response.text()
    throw new Error(`tool bridge ${toolID} failed (${response.status}): ${detail.trim()}`)
  }
  return response.json()
}

export default async () => {
  const definitions = {
    read: tool({
      description: "Read a file or directory from the isolated candidate workspace.",
      args: {
        filePath: tool.schema.string().describe("The absolute path to the file or directory to read"),
        offset: tool.schema.number().int().nonnegative().optional().describe("The line number to start reading from (1-indexed)"),
        limit: tool.schema.number().int().nonnegative().optional().describe("The maximum number of lines to read (defaults to 2000)"),
      },
      async execute(args, context) {
        await context.ask({ permission: "read", patterns: [args.filePath], always: ["*"], metadata: {} })
        return execute("read", args, context.abort)
      },
    }),
    edit: tool({
      description: "Replace text in a file inside the isolated candidate workspace.",
      args: {
        filePath: tool.schema.string().describe("The absolute path to the file to modify"),
        oldString: tool.schema.string().describe("The text to replace"),
        newString: tool.schema.string().describe("The text to replace it with (must be different from oldString)"),
        replaceAll: tool.schema.boolean().optional().describe("Replace all occurrences of oldString (default false)"),
      },
      async execute(args, context) {
        await context.ask({ permission: "edit", patterns: [args.filePath], always: ["*"], metadata: {} })
        return execute("edit", args, context.abort)
      },
    }),
    write: tool({
      description: "Write a file inside the isolated candidate workspace.",
      args: {
        content: tool.schema.string().describe("The content to write to the file"),
        filePath: tool.schema.string().describe("The absolute path to the file to write (must be absolute, not relative)"),
      },
      async execute(args, context) {
        await context.ask({ permission: "edit", patterns: [args.filePath], always: ["*"], metadata: {} })
        return execute("write", args, context.abort)
      },
    }),
    bash: tool({
      description: "Execute a bash command inside the isolated credential-free tool container.",
      args: {
        command: tool.schema.string().describe("The command to execute"),
        timeout: tool.schema.number().int().positive().optional().describe("Optional timeout in milliseconds"),
        workdir: tool.schema.string().optional().describe("The working directory, restricted to /workspace"),
      },
      async execute(args, context) {
        await context.ask({ permission: "bash", patterns: [args.command], always: ["*"], metadata: {} })
        return execute("bash", args, context.abort)
      },
    }),
    apply_patch: tool({
      description: "Apply an OpenCode patch to files inside the isolated candidate workspace.",
      args: {
        patchText: tool.schema.string().describe("The full patch text that describes all changes to be made"),
      },
      async execute(args, context) {
        await context.ask({ permission: "edit", patterns: ["*"], always: ["*"], metadata: {} })
        return execute("apply_patch", args, context.abort)
      },
    }),
  }
  if (process.env.TOOL_BRIDGE_TEST_ALIASES === "1") {
    return {
      tool: {
        ...definitions,
        bridge_test_read: definitions.read,
        bridge_test_edit: definitions.edit,
        bridge_test_write: definitions.write,
        bridge_test_bash: definitions.bash,
        bridge_test_apply_patch: definitions.apply_patch,
      },
    }
  }
  return { tool: definitions }
}
