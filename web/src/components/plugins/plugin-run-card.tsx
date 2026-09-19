import { Button } from "antd";
import { useEffect, useRef, useState } from "react";
import { useUserStore } from "@/stores/use-user-store";
import { cancelPluginRun, decidePluginRun, getPluginRun, type PluginRunView } from "@/services/api/plugin-operations";
import { PluginResultValues } from "./plugin-result-values";
import { PluginCanvasSave } from "./plugin-canvas-save";
import { refreshCanvasAfterAgent, saveRemoteUserDataNow } from "@/services/user-data-sync";

export function PluginRunCard({ runId, canvasId, expectedDigest, viewId }: { runId: string; canvasId?: string; expectedDigest?: string; viewId?: string }) {
    const userID = useUserStore((s) => s.user?.id);
    const scope = `${userID || ""}:${runId}:${viewId || ""}:${expectedDigest || ""}:${canvasId || ""}`;
    const currentScope = useRef(scope);
    currentScope.current = scope;
    const [loaded, setLoaded] = useState<{ scope: string; run: PluginRunView }>();
    const [error, setError] = useState("");
    const [busy, setBusy] = useState(false);
    const [refresh, setRefresh] = useState(0);
    const [projection, setProjection] = useState<{ scope: string; id: string }>();
    const run = loaded?.scope === scope ? loaded.run : undefined;
    const statusLabel = run ? ({ waiting_approval: "等待确认", succeeded: "已完成", cancelled: "已取消", failed: "执行失败", running: "执行中", queued: "排队中" }[run.status] || run.status) : "读取中";
    useEffect(() => {
        const controller = new AbortController();
        setError("");
        setBusy(false);
        void getPluginRun(runId, controller.signal, viewId)
            .then((value) => {
                if (expectedDigest && value.resultRef?.digest !== expectedDigest) throw new Error("结果摘要与节点绑定不符，请重新绑定真实成功结果");
                if (currentScope.current === scope) setLoaded((current) => current?.scope === scope && current.run.revision > value.revision ? current : { scope, run: value });
            })
            .catch((cause) => {
                if (!controller.signal.aborted && currentScope.current === scope) setError(cause instanceof Error ? cause.message : "读取运行失败");
            });
        return () => controller.abort();
    }, [scope, runId, refresh, viewId, expectedDigest]);
    const act = async (action: "approve" | "reject" | "cancel") => {
        if (!run || busy) return;
        setBusy(true);
        setError("");
        try {
            if (canvasId && run.executionAdapter === "canvas.blueprint.instantiate" && action === "approve") await saveRemoteUserDataNow(canvasId);
            if (currentScope.current !== scope) return;
            const value = action === "cancel" ? await cancelPluginRun(run) : await decidePluginRun(run, action);
            if (currentScope.current === scope) setLoaded({ scope, run: value });
            if (currentScope.current === scope && canvasId && value.executionAdapter === "canvas.blueprint.instantiate" && value.status === "succeeded") await refreshCanvasAfterAgent(canvasId);
        } catch (cause) {
            if (currentScope.current === scope) setError(cause instanceof Error ? cause.message : "操作失败，请重新读取状态");
        } finally {
            if (currentScope.current === scope) setBusy(false);
        }
    };
    return (
        <section aria-label="插件运行" className="m-2 space-y-2 rounded-lg border border-border bg-card p-3 text-sm">
            <div className="flex items-center justify-between gap-2">
                <span role="status" aria-live="polite">插件运行：{statusLabel}</span>
                <Button size="small" disabled={busy} onClick={() => setRefresh((v) => v + 1)}>
                    刷新状态
                </Button>
            </div>
            <p className="break-all text-xs text-muted-foreground">{runId}</p>
            {error && (
                <p role="alert" className="text-destructive">
                    {error}
                </p>
            )}
            {run && (
                <>
                    <p className="break-all">{run.operation} · {run.releaseVersion}</p>
                    {run.failureMessage && <p role="alert" className="text-destructive">{run.failureMessage}</p>}
                    <PluginResultValues value={run.result ?? run.preview} view={run.view} />
                    {canvasId && run.status === "succeeded" && run.canvasActions?.map((action) => <PluginCanvasSave key={`${scope}:${action.operation}:${action.blueprintId}`} run={run} canvasId={canvasId} action={action} onCreated={(id) => setProjection({ scope, id })} />)}
                    {canvasId && run.executionAdapter === "canvas.blueprint.instantiate" && run.status === "succeeded" && <Button size="small" onClick={() => void refreshCanvasAfterAgent(canvasId).catch((cause) => setError(cause instanceof Error ? cause.message : "画布同步失败"))}>同步画布结果</Button>}
                </>
            )}
            {run?.status === "waiting_approval" && (
                <div className="flex flex-wrap gap-2">
                    <Button size="small" type="primary" loading={busy} onClick={() => void act("approve")}>
                        {run.executionAdapter === "canvas.blueprint.instantiate" ? "确认保存到画布" : "确认保存快照"}
                    </Button>
                    <Button size="small" disabled={busy} onClick={() => void act("reject")}>
                        拒绝
                    </Button>
                    <Button size="small" disabled={busy} onClick={() => void act("cancel")}>
                        取消运行
                    </Button>
                </div>
            )}
            {projection?.scope === scope && <PluginRunCard key={projection.id} runId={projection.id} canvasId={canvasId} />}
        </section>
    );
}
