import { useUserStore } from "@/stores/use-user-store";
import type { CanvasNodeData } from "@/types/canvas";

// Online editor commands only. No HTTP writes, model calls or fee authority.
export type PluginEditorAction = { canvasId: string; runId: string; nodeId?: string; inputRequestId?: string; command: "editor.focus" | "result.continue" };
export type PluginEditorEvent = PluginEditorAction & { userId: string; error?: string; handled: boolean };
export function pluginActionNode(nodes: CanvasNodeData[], action: PluginEditorAction) {
    return nodes.find((node) => {
        if (action.nodeId && node.id !== action.nodeId) return false;
        const binding = action.inputRequestId ? node.metadata?.pluginInput : node.metadata?.pluginResult;
        return binding?.runId === action.runId && (!action.inputRequestId || node.metadata?.pluginInput?.inputRequestId === action.inputRequestId);
    });
}
export function requestPluginEditorAction(action: PluginEditorAction) {
    const userId = useUserStore.getState().user?.id;
    if (!userId) throw new Error("请先登录");
    const detail: PluginEditorEvent = { ...action, userId, handled: false };
    window.dispatchEvent(new CustomEvent("qimu:plugin-editor-action", { detail }));
    if (!detail.handled || detail.error) throw new Error(detail.error || "请先打开对应画布");
}
