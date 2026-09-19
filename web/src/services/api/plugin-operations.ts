import { http } from "@/services/api/request";
import type { PluginInvocation, PluginOperation, PluginResultRef } from "@/lib/plugins/plugin-v3-types";

export type OperationDescription = { operation: string; releaseId: string; contractHash: string; definition: PluginOperation; schemas: Record<string, unknown>; available: boolean; reason?: string };
export type InvocationOutput = { kind: "inline"; result: unknown } | { kind: "run"; runId: string; status: string; revision: number; approvalId: string };
export type PluginRunView = { id: string; operation: string; status: string; revision: number; approvalId?: string; sourceResourceId?: string; preview?: unknown; result?: unknown; resultRef?: PluginResultRef };
export function describePluginOperation(pluginId: string, operationId: string, releaseId: string) {
    return http.get<OperationDescription>(`/plugin-operations/${encodeURIComponent(pluginId)}/${encodeURIComponent(operationId)}`, { params: { releaseId } });
}
export function invokePluginOperation(request: PluginInvocation, key: string) {
    return http.post<InvocationOutput>("/plugin-invocations", request, { headers: { "Idempotency-Key": key } });
}
export function getPluginRun(id: string, signal?: AbortSignal) {
    return http.get<PluginRunView>(`/plugin-runs/${encodeURIComponent(id)}`, { signal });
}
export function decidePluginRun(run: PluginRunView, decision: "approve" | "reject") {
    return http.post<PluginRunView>(`/plugin-runs/${encodeURIComponent(run.id)}/approvals/${encodeURIComponent(run.approvalId || "")}`, { decision, revision: run.revision });
}
export function cancelPluginRun(run: PluginRunView) {
    return http.post<PluginRunView>(`/plugin-runs/${encodeURIComponent(run.id)}/cancel`, { revision: run.revision });
}
