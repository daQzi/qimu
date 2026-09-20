import { http } from "./request";
import type { AgentRun, CreateAgentRunInput } from "./agent";

export type AgentThread = { id: string; title: string; canvasId?: string; revision: number; contextRevision: number; lastSequence: number; lastAgentRunId?: string; createdAt: string; updatedAt: string };
export type ThreadReceipt = { kind: "plugin" | "creation"; runId: string; status: string };
export type AgentThreadEntry = { threadId: string; sequence: number; kind: "agent" | "plugin" | "creation"; runId: string; prompt: string; contextRevision: number; context?: CreateAgentRunInput; run?: AgentRun; references: ThreadReceipt[]; createdAt: string };
export type AgentThreadView = { thread: AgentThread; entries: AgentThreadEntry[]; canvases: Array<{ canvasId: string }>; nextBefore?: number };
const path = (id: string) => `/agent/threads/${encodeURIComponent(id)}`;
export const listAgentThreads = (canvasId = "", offset = 0) => http.get<{ threads: AgentThread[]; hasMore: boolean }>("/agent/threads", { params: { canvasId, offset } });
export const createAgentThread = (clientKey: string, canvasId = "") => http.post<{ thread: AgentThread }>("/agent/threads", { clientKey, canvasId });
export const getAgentThread = (id: string, before = 0) => http.get<AgentThreadView>(path(id), { params: { before } });
export const bindAgentThread = (id: string, revision: number, canvasId: string) => http.patch<{ thread: AgentThread }>(`${path(id)}/context`, { revision, canvasId });
export const appendAgentThreadMessage = (id: string, revision: number, request: CreateAgentRunInput) => http.post<{ thread: AgentThread; run: AgentRun }>(`${path(id)}/messages`, { revision, request });
export const appendAgentThreadReference = (id: string, revision: number, kind: ThreadReceipt["kind"], runId: string, clientKey: string) => http.post<{ thread: AgentThread }>(`${path(id)}/references`, { revision, kind, runId, clientKey });
