import { http } from "./request";
import type { PluginRunView } from "./plugin-operations";

export type PluginConnection = { id?: string; pluginId: string; connectorId: string; name: string; baseUrl: string; enabled: boolean; revision: number; credentialConfigured?: boolean; authType?: string };
export type PluginOperationPrice = { releaseId: string; operationId: string; feeMicrocredits: number; revision: number };
export function getPluginConnections(signal?: AbortSignal) {
    return http.get<{ connections: PluginConnection[] }>("/plugin-connections", { signal });
}
export function savePluginConnection(value: PluginConnection, credential: string) {
    return http.put<PluginConnection>("/plugin-connections", { pluginId: value.pluginId, connectorId: value.connectorId, name: value.name, baseUrl: value.baseUrl, enabled: value.enabled, revision: value.revision, credential });
}
export function getPluginPrice(releaseId: string, operationId: string, signal?: AbortSignal) {
    return http.get<PluginOperationPrice>(`/admin/plugin-operation-prices/${encodeURIComponent(releaseId)}/${encodeURIComponent(operationId)}`, { signal });
}
export function savePluginPrice(value: PluginOperationPrice) {
    return http.put<PluginOperationPrice>("/admin/plugin-operation-prices", value);
}
export function resumePluginRun(run: PluginRunView, action: "retry_safe" | "attach_job" | "retry_import" | "retry_poll", providerJobId?: string) {
    return http.post<PluginRunView>(`/plugin-runs/${encodeURIComponent(run.id)}/resume`, { revision: run.revision, action, ...(providerJobId ? { providerJobId } : {}) });
}
