import { Button } from "antd";
import { useRef, useState } from "react";
import type { PluginInvocation } from "@/lib/plugins/plugin-v3-types";
import { describePluginOperation, getPluginCanvasSnapshot, invokePluginOperation, type PluginCanvasAction, type PluginRunView } from "@/services/api/plugin-operations";
import { saveRemoteUserDataNow } from "@/services/user-data-sync";
import { useUserStore } from "@/stores/use-user-store";

export function PluginCanvasSave({ run, canvasId, action, onCreated }: { run: PluginRunView; canvasId: string; action: PluginCanvasAction; onCreated: (id: string) => void }) {
    const userID = useUserStore((s) => s.user?.id);
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState("");
    const attempt = useRef<{ request: PluginInvocation; key: string } | undefined>(undefined);
    const save = async (newInstance: boolean) => {
        if (busy || (!run.resultRef && !action.inputRequestId)) return;
        setBusy(true);
        setError("");
        try {
            await saveRemoteUserDataNow(canvasId);
            if (useUserStore.getState().user?.id !== userID) return;
            if (!attempt.current || newInstance) {
                const snapshot = await getPluginCanvasSnapshot(canvasId);
                if (useUserStore.getState().user?.id !== userID) return;
                const [pluginID, operationID] = action.operation.split(".");
                await describePluginOperation(pluginID, operationID, action.releaseId, { hostSurface: "canvas", canvasId });
                if (useUserStore.getState().user?.id !== userID) return;
                attempt.current = {
                    key: crypto.randomUUID(),
                    request: {
                        operation: action.operation,
                        releaseId: action.releaseId,
                        context: { hostSurface: "canvas", canvasId },
                        input: {
                            runId: run.id,
                            ...(action.inputRequestId ? { inputRequestId: action.inputRequestId } : { resultDigest: run.resultRef!.digest }),
                            blueprintId: action.blueprintId,
                            snapshotHash: snapshot.snapshotHash,
                            instanceKey: newInstance ? crypto.randomUUID() : "default",
                        },
                    },
                };
            }
            const output = await invokePluginOperation(attempt.current.request, attempt.current.key);
            if (useUserStore.getState().user?.id !== userID) return;
            if (output.kind !== "run") throw new Error("画布保存未返回审批运行");
            onCreated(output.runId);
            attempt.current = undefined;
        } catch (cause) {
            if (useUserStore.getState().user?.id === userID) setError(cause instanceof Error ? cause.message : "保存失败，可重试同一请求");
        } finally {
            setBusy(false);
        }
    };
    return (
        <div className="space-y-2">
            <div className="flex flex-wrap gap-2">
                <Button size="small" loading={busy} onClick={() => void save(false)}>
                    {action.inputRequestId ? "创建画布输入节点" : "保存到当前画布"}
                </Button>
                <Button size="small" disabled={busy} onClick={() => void save(true)}>
                    {action.inputRequestId ? "另建输入节点" : "另建结果节点"}
                </Button>
                {error && (
                    <Button
                        size="small"
                        disabled={busy}
                        onClick={() => {
                            attempt.current = undefined;
                            setError("");
                        }}
                    >
                        重新读取画布后再试
                    </Button>
                )}
            </div>
            {error && (
                <p role="alert" className="text-destructive">
                    {error}
                </p>
            )}
            <p className="text-xs text-muted-foreground">移动、删除或撤销节点只改变画布展示；不会停止运行或撤销费用。冲突后可重新读取画布并另建节点。</p>
        </div>
    );
}
