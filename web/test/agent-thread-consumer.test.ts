import { expect, test } from "bun:test";
import { applyAgentEvent, replayThreadMessages, threadAgentEvent } from "@/services/agent-event-consumer";
import type { AgentThreadEntry } from "@/services/api/agent-threads";
import type { AgentEvent, AgentRun } from "@/services/api/agent";

function entry(id: string, sequence: number, text: string): AgentThreadEntry {
    const events: AgentEvent[] = [{ eventId: `${id}:1`, runId: id, seq: 1, type: "assistant_message", payload: { messageId: "assistant", text }, createdAt: "2026-09-20" }];
    return { threadId: "thread", sequence, kind: "agent", runId: id, prompt: `prompt ${sequence}`, contextRevision: sequence, run: { id, events, status: "completed" } as AgentRun, references: [], createdAt: "2026-09-20" };
}

test("durable history keeps identically named messages in different turns separate", () => {
    const messages = replayThreadMessages([entry("home", 1, "home reply"), entry("canvas", 2, "canvas reply")]);
    expect(messages.map((message) => message.text)).toEqual(["prompt 1", "home reply", "prompt 2", "canvas reply"]);
    expect(new Set(messages.map((message) => message.id)).size).toBe(4);
});

test("snapshot replay and live consumer use the same message identity", () => {
    const item = entry("run-one", 1, "reply");
    let messages = replayThreadMessages([item]);
    applyAgentEvent(threadAgentEvent(item.run!.events![0]), (next) => { messages = typeof next === "function" ? next(messages) : next; }, () => undefined, () => undefined);
    expect(messages).toHaveLength(2);
    expect(messages[1].text).toBe("reply");
});

test("reading historical approvals does not request an approval or replay canvas writes", () => {
    const item = entry("run", 1, "reply");
    item.run!.events!.push({ runId: "run", eventId: "run:2", seq: 2, type: "approval_requested", payload: { approvalId: "approval" }, createdAt: "2026-09-20" });
    expect(replayThreadMessages([item]).map((message) => message.text)).toEqual(["prompt 1", "reply"]);
    expect(threadAgentEvent(item.run!.events![0]).seq).toBe(1);
});
