import { requestToolResponse, type ResponseFunctionTool, type ResponseInputMessage, type ToolChoice } from "@/services/api/image";
import { describePluginOperation, invokePluginOperation } from "@/services/api/plugin-operations";
import type { PluginInvocationContext } from "@/lib/plugins/plugin-v3-types";

// Applications bind an immutable release. Authorization and execution remain
// server-side; this bridge never uses the legacy browser AI credentials.
export function createApplicationPluginHost(pluginId: string, releaseId: string, context?: PluginInvocationContext) {
    return {
        describe: (operationId: string) => describePluginOperation(pluginId, operationId, releaseId, context),
        invoke: (operationId: string, input: Record<string, unknown>, key: string) => invokePluginOperation({ operation: `${pluginId}.${operationId}`, releaseId, input, ...(context ? { context } : {}) }, key),
    };
}
import { pluginStorageFor } from "@/lib/plugins/plugin-storage";
import type { AiConfig } from "@/stores/use-config-store";
import type { PluginHostContext, PluginInstallation, PluginTextRequest, RegisteredPlugin } from "@/lib/plugins/plugin-types";

export function createPluginHostContext(plugin: RegisteredPlugin, installation: PluginInstallation, aiConfig: AiConfig): PluginHostContext {
    const permissions = new Set(plugin.manifest.permissions);
    return {
        manifest: plugin.manifest,
        permissions,
        storage: pluginStorageFor(plugin.manifest.id),
        config: installation.config,
        services: {
            ai: {
                text: {
                    requestToolResponse: async (request: PluginTextRequest) => {
                        if (!permissions.has("ai.text")) throw new Error("插件没有调用文本模型的权限");
                        const response = await requestToolResponse(
                            { ...aiConfig, model: request.model?.trim() || aiConfig.textModel },
                            request.messages as ResponseInputMessage[],
                            (request.tools || []) as ResponseFunctionTool[],
                            (request.toolChoice || "auto") as ToolChoice,
                            request.onDelta,
                            { signal: request.signal },
                        );
                        return {
                            content: response.content,
                            toolCalls: response.toolCalls.map((call) => ({ name: call.function.name, arguments: call.function.arguments })),
                        };
                    },
                },
            },
        },
    };
}
