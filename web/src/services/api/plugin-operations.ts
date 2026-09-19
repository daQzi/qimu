import { http } from "@/services/api/request";
import type { PluginInvocation, PluginInvocationContext, PluginOperation, PluginResultRef } from "@/lib/plugins/plugin-v3-types";

export type OperationDescription = { operation: string; releaseId: string; contractHash: string; definition: PluginOperation; schemas: Record<string, unknown>; available: boolean; reason?: string; resultView?: PluginResultView };
export type InvocationOutput = { kind: "inline"; result: unknown; digest: string } | { kind: "run"; runId: string; status: string; revision: number; approvalId: string };
export type PluginResultView = { id: string; component: "key-value/v1"; fields: { path: string; label: string }[] };
export type PluginCanvasAction = { operation: string; releaseId: string; blueprintId: string };
export type PluginRunView = { id: string; operation: string; releaseId: string; releaseVersion: string; status: string; revision: number; approvalId?: string; sourceResourceId?: string; preview?: unknown; result?: unknown; resultRef?: PluginResultRef; view?: PluginResultView; canvasActions?: PluginCanvasAction[]; executionAdapter?: string; projectionStatus?: string; failureMessage?: string; taskId?: string; remote?: { taskId: string; submissionState: string; providerJobId?: string; cancelStatus?: string; cancelRequested: boolean; failureReason?: string; failureMessage?: string; importAttempts: number } };
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
