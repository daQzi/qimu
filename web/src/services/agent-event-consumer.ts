import type { Dispatch, SetStateAction } from "react";
import type { AgentRun, AgentEvent } from "./api/agent";
import type { CloudAgentChatMessage, CloudAgentPlanItem } from "@/components/canvas/canvas-cloud-agent-chat-ui";
import { agentErrorPresentation } from "@/lib/canvas/agent-error-presentation";
export type ApprovalState = { approvalId: string; detail: Record<string, unknown>; reason: string };

export function threadAgentEvent(event: AgentEvent): AgentEvent {
    if (!event.payload?.messageId || (!event.type.startsWith("assistant_") && !event.type.startsWith("reasoning_"))) return event;
    return { ...event, payload: { ...event.payload, messageId: `${event.runId}:${event.payload.messageId}` } };
}

export function replayThreadMessages(entries: import("./api/agent-threads").AgentThreadEntry[]) {
    let messages: CloudAgentChatMessage[] = [];
    const update: Dispatch<SetStateAction<CloudAgentChatMessage[]>> = (next) => { messages = typeof next === "function" ? next(messages) : next; };
    for (const entry of entries) {
        if (entry.kind !== "agent") continue;
        messages = appendUniqueMessage(messages, { id: `thread-user-${entry.runId}`, role: "user", text: entry.prompt });
        for (const event of entry.run?.events || []) applyAgentEvent(threadAgentEvent(event), update, () => undefined, () => undefined);
        if (entry.run?.activeMessage?.text) {
            const active = entry.run.activeMessage;
            messages = upsertTextMessage(messages, `${entry.runId}:${active.messageId || "active"}`, active.text, false);
        }
    }
    return messages;
}

export function applyAgentEvent(event: AgentEvent, setMessages: Dispatch<SetStateAction<CloudAgentChatMessage[]>>, setRun: Dispatch<SetStateAction<AgentRun | null>>, setApproval: Dispatch<SetStateAction<ApprovalState | null>>, setPrompt?: Dispatch<SetStateAction<string>>) {
    const payload = event.payload || {};
    const text = String(payload.text || payload.summary || payload.message || "");
    if (event.type === "run_status") {
        const snapshotApproval = payload.approval && typeof payload.approval === "object" ? payload.approval as AgentRun["approval"] : undefined;
        const pendingExecution = payload.pendingExecution as AgentRun["pendingExecution"];
        setRun((current) => (current ? { ...current, status: String(payload.status || current.status) as AgentRun["status"], updatedAt: event.createdAt, revision: Number(payload.revision || 0), cleanupPending: Boolean(payload.cleanupPending), failureMessage: String(payload.failureMessage || ""), skills: payload.skills as AgentRun["skills"], spentCredits: Number(payload.spentCredits || 0), step: Number(payload.step || 0), approval: snapshotApproval, pendingExecution } : current));
        if (payload.failureMessage) setMessages((current) => appendAgentError(current, `terminal-${event.runId}`, String(payload.failureMessage)));
        if (snapshotApproval && !snapshotApproval.decision && snapshotApproval.approvalId) {
            setApproval((current) => ({ approvalId: snapshotApproval.approvalId, detail: snapshotApproval, reason: current?.approvalId === snapshotApproval.approvalId ? current.reason : snapshotApproval.reason || "" }));
        } else {
            setApproval(null);
        }
        return;
    }
    if (event.type === "approval_decided") {
        setApproval(null);
        if (payload.decision === "reject") {
            setMessages((current) => appendUniqueMessage(current, {
                id: event.eventId,
                role: "system",
                text: text || "已拒绝本次操作，未写入画布。你可以告诉 Agent 修改方向后重新申请。",
            }));
        }
        return;
    }
    if (event.type === "progress_summary") {
        setMessages((current) => appendUniqueMessage(current, { id: event.eventId, role: "system", text: text || "Agent 正在整理执行计划" }));
        return;
    }
    if (event.type === "assistant_delta") {
        setMessages((current) => upsertTextMessage(current, String(payload.messageId || "assistant"), text, true));
        return;
    }
    if (event.type === "reasoning_delta" || event.type === "reasoning_message") {
        const id = String(payload.messageId || `${event.runId}:reasoning`);
        setMessages((current) => upsertTextMessage(current, id, text, event.type === "reasoning_delta")
            .map((item) => item.id === id ? { ...item, reasoning: true } : item));
        return;
    }
    if (event.type === "plan_updated" && Array.isArray(payload.items)) {
        const id = `plan-${event.runId}`;
        const planItems = payload.items as CloudAgentPlanItem[];
        setMessages((current) => {
            const index = current.findIndex((entry) => entry.id === id);
            const message: CloudAgentChatMessage = { id, role: "tool", text: "", planItems };
            if (index < 0) return [...current, message];
            const next = [...current];
            next[index] = message;
            return next;
        });
        return;
    }
    if (event.type === "user_interjection") {
        if (!text) return;
        setMessages((current) => appendUniqueMessage(current, { id: String(payload.messageId || event.eventId), role: "user", text, interjection: "sent" }));
        return;
    }
    if (event.type === "user_interjection_dropped") {
        const messageId = String(payload.messageId || event.eventId);
        const reason = String(payload.reason || "本轮已结束");
        setMessages((current) => {
            const marked = current.map((item) => item.id === messageId ? { ...item, interjection: "undelivered" as const } : item);
            return appendUniqueMessage(marked, { id: `interjection-dropped-${messageId}`, role: "system", text: `${reason}，这条插话没有送到模型。需要的话重新发一次，它会作为新一轮。` });
        });
        setPrompt?.((current) => (current.trim() ? current : text));
        return;
    }
    if (event.type === "user_question") {
        const options = Array.isArray(payload.options)
            ? (payload.options as Array<{ label?: unknown; detail?: unknown }>)
                .map((option) => ({ label: String(option?.label || "").trim(), detail: option?.detail === undefined ? undefined : String(option.detail) }))
                .filter((option) => option.label)
            : [];
        const question = String(payload.question || "").trim();
        if (!question || options.length < 2) return;
        const id = `question-${event.runId}:${event.seq ?? event.eventId}`;
        setMessages((current) => appendUniqueMessage(current, {
            id,
            role: "assistant",
            text: "",
            question: { question, options, allowFreeform: payload.allowFreeform !== false },
        }));
        return;
    }
    if (event.type === "assistant_message") {
        setMessages((current) => upsertTextMessage(current, String(payload.messageId || event.eventId), text, false));
        return;
    }
    if (event.type === "assistant_snapshot") {
        setMessages((current) => upsertTextMessage(current, String(payload.messageId || event.eventId), text, false));
        return;
    }
    if (event.type === "approval_requested") {
        const approvalId = String(payload.approvalId || "");
        setApproval((current) => ({ approvalId, detail: payload, reason: current?.approvalId === approvalId ? current.reason : "" }));
        return;
    }
    if (event.type === "canvas_updated" && Array.isArray(payload.actions)) {
        if (payload.operation === "generate_media_submit" || payload.operation === "generate_media_complete") return;
        const { canvasPatch: _patch, ...detail } = payload;
        const id = payload.callId ? `canvas-${event.runId}-${payload.callId}` : event.eventId;
        setMessages((current) => appendUniqueMessage(current, { id, role: "tool", title: "canvas_apply_ops", text: "画布操作已完成", detail: { ...detail, eventType: event.type } }));
        return;
    }
    if (event.type === "tool_completed" && payload.toolName === "canvas_apply_ops" && payload.callId) {
        const id = `canvas-${event.runId}-${payload.callId}`;
        setMessages((current) => appendUniqueMessage(current, { id, role: "tool", title: "canvas_apply_ops", text: text || "画布操作已完成", detail: { ...payload, eventType: event.type } }));
        return;
    }
    if (event.type === "generation_task_created") {
        const message: CloudAgentChatMessage = { id: event.eventId, role: "tool", title: "generate_media", text: text || event.type, detail: { ...payload, eventType: event.type } };
        setMessages((current) => upsertMediaToolTrace(current, message));
        return;
    }
    if (event.type.startsWith("tool_")) {
        const message: CloudAgentChatMessage = { id: event.eventId, role: "tool", title: String(payload.toolName || payload.title || "工具执行"), text: text || event.type, detail: { ...payload, eventType: event.type } };
        if (payload.toolName === "generate_media") {
            setMessages((current) => upsertMediaToolTrace(current, message));
        } else {
            setMessages((current) => appendUniqueMessage(current, message));
        }
        return;
    }
    if (event.type === "run_failed" || event.type === "error") setMessages((current) => appendAgentError(current, event.eventId, text || "Agent 执行失败"));
}
function toolDetailRecord(value: unknown): Record<string, unknown> {
    return value && typeof value === "object" && !Array.isArray(value) ? value as Record<string, unknown> : {};
}

function toolDetailNodeIds(detail: unknown): Set<string> {
    const payload = toolDetailRecord(detail);
    const ids = new Set<string>();
    for (const value of [payload.nodeId, toolDetailRecord(payload.result).nodeId]) {
        if (typeof value === "string" && value) ids.add(value);
    }
    if (Array.isArray(payload.actions)) {
        for (const action of payload.actions) {
            const nodeId = toolDetailRecord(action).nodeId;
            if (typeof nodeId === "string" && nodeId) ids.add(nodeId);
        }
    }
    if (typeof payload.arguments === "string") {
        try {
            const args = toolDetailRecord(JSON.parse(payload.arguments));
            if (Array.isArray(args.ops)) {
                for (const op of args.ops) {
                    const nodeId = toolDetailRecord(op).id;
                    if (typeof nodeId === "string" && nodeId) ids.add(nodeId);
                }
            }
        } catch {
            // Tool arguments are diagnostic data; a malformed value must not break the event feed.
        }
    }
    return ids;
}

function toolDetailTaskIds(detail: unknown): Set<string> {
    const payload = toolDetailRecord(detail);
    const ids = new Set<string>();
    for (const value of [payload.taskId, toolDetailRecord(payload.result).taskId]) {
        if (typeof value === "string" && value) ids.add(value);
    }
    return ids;
}

function mergeToolDetails(previous: unknown, next: unknown): Record<string, unknown> {
    const previousDetail = toolDetailRecord(previous);
    const nextDetail = toolDetailRecord(next);
    return {
        ...previousDetail,
        ...nextDetail,
        actions: Array.isArray(nextDetail.actions) ? nextDetail.actions : previousDetail.actions,
        arguments: nextDetail.arguments || previousDetail.arguments,
    };
}

function upsertMediaToolTrace(current: CloudAgentChatMessage[], message: CloudAgentChatMessage): CloudAgentChatMessage[] {
    const nextNodeIds = toolDetailNodeIds(message.detail);
    const nextTaskIds = toolDetailTaskIds(message.detail);
    const index = current.findIndex((item) => {
        if (item.role !== "tool") return false;
        const itemToolName = item.title || "";
        if (itemToolName !== "canvas_apply_ops" && itemToolName !== "generate_media") return false;
        const itemNodeIds = toolDetailNodeIds(item.detail);
        const itemTaskIds = toolDetailTaskIds(item.detail);
        return [...nextNodeIds].some((id) => itemNodeIds.has(id)) || [...nextTaskIds].some((id) => itemTaskIds.has(id));
    });
    if (index < 0) return appendUniqueMessage(current, message);
    const next = [...current];
    const previous = next[index];
    next[index] = {
        ...previous,
        ...message,
        id: previous.id,
        detail: mergeToolDetails(previous.detail, message.detail),
    };
    return next;
}

export function appendUniqueMessage(current: CloudAgentChatMessage[], message: CloudAgentChatMessage) {
    return current.some((item) => item.id === message.id) ? current : [...current, message];
}
function upsertTextMessage(current: CloudAgentChatMessage[], id: string, text: string, append: boolean): CloudAgentChatMessage[] {
    const index = current.findIndex((item) => item.id === id);
    if (index < 0) return [...current, { id, role: "assistant" as const, text, streaming: append }];
    if (!append && current[index].text === text && !current[index].streaming) return current;
    const next = [...current];
    next[index] = { ...next[index], text: append ? `${next[index].text}${text}` : text, streaming: append };
    return next;
}

export function appendAgentError(current: CloudAgentChatMessage[], id: string, cause: unknown, fallback?: string) {
    const message = agentErrorPresentation(cause, fallback);
    const last = current.at(-1);
    if (last?.role === "error" && last.title === message.title && last.text === message.text) return current;
    return appendUniqueMessage(current, { id, role: "error", ...message });
}
