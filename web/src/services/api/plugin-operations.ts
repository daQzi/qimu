import { http } from "@/services/api/request";
import type { PluginInvocation, PluginInvocationContext, PluginOperation, PluginResultRef } from "@/lib/plugins/plugin-v3-types";

export type OperationDescription = { operation: string; releaseId: string; contractHash: string; definition: PluginOperation; schemas: Record<string, unknown>; available: boolean; reason?: string; resultView?: PluginResultView };
export type InvocationOutput = { kind: "inline"; result: unknown; digest: string } | { kind: "run"; runId: string; status: string; revision: number; approvalId: string };
export type PluginResultView = { id: string; component: "key-value/v1"; fields: { path: string; label: string }[] };
export type PluginCanvasAction = { operation: string; releaseId: string; blueprintId: string };
export type PluginInputRequest = { id: string; runId: string; stepKey: string; revision: number; status: string; schema: Record<string, unknown>; draft?: unknown; submitted?: unknown };
export type PluginRunView = {
    id: string;
    operation: string;
    releaseId: string;
    releaseVersion: string;
    status: string;
    revision: number;
    eventSequence?: number;
    approvalId?: string;
    sourceResourceId?: string;
    preview?: unknown;
    result?: unknown;
    resultRef?: PluginResultRef;
    view?: PluginResultView;
    canvasActions?: PluginCanvasAction[];
    executionAdapter?: string;
    projectionStatus?: string;
    failureMessage?: string;
    taskId?: string;
    derivedFromRunId?: string;
    attempt?: number;
    pipeline?: {
        cursor: number;
        steps: { key: string; type: string }[];
        childRunId?: string;
        inputs: PluginInputRequest[];
        outputs: Record<string, unknown>;
        schemas?: Record<string, unknown>;
        batch?: { steps: Record<string, { done: boolean; items: { itemKey: string; status: string; childRunId?: string; reusedFrom?: string }[] }>; blockedReason?: string };
        stepRuns?: PluginRunView[];
    };
    remote?: { taskId: string; submissionState: string; providerJobId?: string; cancelStatus?: string; cancelRequested: boolean; failureReason?: string; failureMessage?: string; importAttempts: number };
};
export type PluginBatchQuote = { digest: string; items: { runId: string; operation: string; feeMicrocredits: number }[]; amountMicrocredits: number; vendorCostKnown: boolean; expiresAt: string };
export function getPluginBatchQuote(id: string) {
    return http.get<PluginBatchQuote>(`/plugin-runs/${encodeURIComponent(id)}/batch-quote`);
}
export function approvePluginBatch(id: string, quote: PluginBatchQuote, acceptExternalBilling: boolean) {
    return http.post<PluginRunView>(`/plugin-runs/${encodeURIComponent(id)}/batch-approval`, { digest: quote.digest, count: quote.items.length, amountMicrocredits: quote.amountMicrocredits, expiresAt: quote.expiresAt, acceptExternalBilling });
}
export function derivePluginRun(id: string, request: { input?: Record<string, unknown>; inputs: Record<string, unknown>; forceSteps: string[]; reuseCompleted: boolean }, key: string) {
    return http.post<PluginRunView>(`/plugin-runs/${encodeURIComponent(id)}/derive`, request, { headers: { "Idempotency-Key": key } });
}
export function listPluginRuns(offset = 0, signal?: AbortSignal) {
    return http.get<PluginRunView[]>("/plugin-runs", { params: { offset }, signal });
}
export function updatePluginInput(runId: string, input: PluginInputRequest, value: unknown, mode: "draft" | "submit", key: string) {
    return http.put<PluginRunView>(`/plugin-runs/${encodeURIComponent(runId)}/inputs/${encodeURIComponent(input.id)}`, { revision: input.revision, value, mode }, { headers: { "Idempotency-Key": key } });
}
export function describePluginOperation(pluginId: string, operationId: string, releaseId: string, context?: PluginInvocationContext) {
    return http.get<OperationDescription>(`/plugin-operations/${encodeURIComponent(pluginId)}/${encodeURIComponent(operationId)}`, { params: { releaseId, ...context } });
}
export function invokePluginOperation(request: PluginInvocation, key: string) {
    return http.post<InvocationOutput>("/plugin-invocations", request, { headers: { "Idempotency-Key": key } });
}
export function getPluginRun(id: string, signal?: AbortSignal, viewId?: string) {
    return http.get<PluginRunView>(`/plugin-runs/${encodeURIComponent(id)}`, { signal, params: { viewId } });
}
export function getPluginCanvasSnapshot(canvasId: string) {
    return http.get<{ canvasId: string; snapshotHash: string }>(`/plugin-canvases/${encodeURIComponent(canvasId)}/snapshot`);
}
export function decidePluginRun(run: PluginRunView, decision: "approve" | "reject") {
    return http.post<PluginRunView>(`/plugin-runs/${encodeURIComponent(run.id)}/approvals/${encodeURIComponent(run.approvalId || "")}`, { decision, revision: run.revision });
}
export function cancelPluginRun(run: PluginRunView) {
    return http.post<PluginRunView>(`/plugin-runs/${encodeURIComponent(run.id)}/cancel`, { revision: run.revision });
}
