import { Button } from "antd";
import { useEffect, useRef, useState } from "react";
import { useUserStore } from "@/stores/use-user-store";
import { cancelPluginRun, decidePluginRun, getPluginRun, type PluginRunView } from "@/services/api/plugin-operations";
import { PluginResultValues } from "./plugin-result-values";
import { PluginCanvasSave } from "./plugin-canvas-save";
import { PluginRemoteStatus } from "./plugin-remote-status";
import { PluginInputForm } from "./plugin-input-form";
import { requestPluginEditorAction } from "@/lib/plugins/plugin-editor-actions";
import { PluginBatchControls } from "./plugin-batch-controls";
import { resumePluginRun } from "@/services/api/plugin-remote";
import { apiBaseURL } from "@/services/api/request";
import { refreshCanvasAfterAgent, saveRemoteUserDataNow } from "@/services/user-data-sync";

export function PluginRunCard({ runId, canvasId, expectedDigest, viewId, inputRequestId, expectedReleaseId }: { runId: string; canvasId?: string; expectedDigest?: string; viewId?: string; inputRequestId?: string; expectedReleaseId?: string }) {
    const userID = useUserStore((s) => s.user?.id);
    const scope = `${userID || ""}:${runId}:${viewId || ""}:${expectedDigest || ""}:${canvasId || ""}:${inputRequestId || ""}:${expectedReleaseId || ""}`;
    const currentScope = useRef(scope);
    currentScope.current = scope;
    const [loaded, setLoaded] = useState<{ scope: string; run: PluginRunView }>();
    const [error, setError] = useState("");
    const [busy, setBusy] = useState(false);
    const [refresh, setRefresh] = useState(0);
    const [projection, setProjection] = useState<{ scope: string; id: string }>();
    const run = loaded?.scope === scope ? loaded.run : undefined;
    useEffect(() => {
        setBusy(false);
    }, [scope]);
    const active = !!run && !["succeeded", "failed", "cancelled"].includes(run.status);
    useEffect(() => {
        if (!active || busy) return;
        const timer = setInterval(() => setRefresh((v) => v + 1), 3000);
        return () => clearInterval(timer);
    }, [scope, active, busy]);
    useEffect(() => {
        if (!active || !run?.pipeline || typeof EventSource === "undefined") return;
        const events = new EventSource(`${apiBaseURL.replace(/\/$/, "")}/plugin-runs/${encodeURIComponent(runId)}/events?after=${run.eventSequence || 0}`, { withCredentials: true });
        events.addEventListener("plugin-run", () => {
            if (currentScope.current === scope) setRefresh((v) => v + 1);
        });
        return () => events.close();
    }, [scope, active, !!run?.pipeline, runId]);
    const statusLabel = run
        ? { waiting_input: "等待填写", waiting_approval: "等待确认", succeeded: "已完成", cancelled: "已取消", failed: "执行失败", running: "执行中", queued: "排队中", paused: "待恢复", cancelling: "正在停止" }[run.status] || run.status
        : "读取中";
    useEffect(() => {
        if (run?.remote && ["running", "succeeded", "failed", "cancelled"].includes(run.status)) window.dispatchEvent(new CustomEvent("wallet:updated"));
    }, [scope, run?.status, !!run?.remote]);
    useEffect(() => {
        const controller = new AbortController();
        setError("");
        void getPluginRun(runId, controller.signal, viewId)
            .then((value) => {
                if (expectedDigest && value.resultRef?.digest !== expectedDigest) throw new Error("结果摘要与节点绑定不符，请重新绑定真实成功结果");
                if (expectedReleaseId && value.releaseId !== expectedReleaseId) throw new Error("节点发布绑定与运行不符");
                if (inputRequestId && !value.pipeline?.inputs.some((input) => input.id === inputRequestId && input.runId === runId)) throw new Error("输入请求不属于该运行");
                if (currentScope.current === scope) setLoaded((current) => (current?.scope === scope && current.run.revision > value.revision ? current : { scope, run: value }));
            })
            .catch((cause) => {
                if (!controller.signal.aborted && currentScope.current === scope) setError(cause instanceof Error ? cause.message : "读取运行失败");
            });
        return () => controller.abort();
    }, [scope, runId, refresh, viewId, expectedDigest, inputRequestId, expectedReleaseId]);
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
                <span role="status" aria-live="polite">
                    插件运行：{statusLabel}
                </span>
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
                    <p className="break-all">
                        {run.operation} · {run.releaseVersion}
                    </p>
                    {run.workbench ? <details><summary>查看本次工作台输入</summary><p className="break-all">{run.workbench.id} · {run.workbench.releaseId}</p><p>配方：{run.workbench.recipeIds.join("、") || "无"}</p><PluginResultValues value={run.workbench.input} /></details> : null}
                    {run.failureMessage && (
                        <p role="alert" className="text-destructive">
                            {run.failureMessage}
                        </p>
                    )}
                    {run.pipeline && (
                        <>
                            {inputRequestId && run.pipeline.inputs.find((input) => input.id === inputRequestId)?.status === "submitted" && <p role="status">此节点的输入已提交，后台流程将继续；无需重复填写。</p>}
                            {canvasId && !inputRequestId && run.pipeline.inputs.some((input) => input.status === "pending") && <Button size="small" onClick={() => {
                                try { requestPluginEditorAction({ command: "editor.focus", canvasId, runId, inputRequestId: run.pipeline!.inputs.find((input) => input.status === "pending")!.id }); }
                                catch (cause) { setError(cause instanceof Error ? cause.message : "定位失败"); }
                            }}>定位下一个待填写节点</Button>}
                            <PluginBatchControls
                                key={scope}
                                run={run}
                                onUpdated={(value) => {
                                    if (currentScope.current === scope) setLoaded({ scope, run: value });
                                }}
                                onDerived={(id) => {
                                    if (currentScope.current === scope) setProjection({ scope, id });
                                }}
                            />
                            <p>
                                已完成 {run.pipeline.cursor} / {run.pipeline.steps.length} 步
                            </p>
                            <ol className="list-inside list-decimal">
                                {run.pipeline.steps.map((step, i) => (
                                    <li key={step.key}>
                                        {step.key}：{run.pipeline!.batch ? (run.pipeline!.batch.steps[step.key]?.done ? "已完成" : "待完成") : i < run.pipeline!.cursor ? "已完成" : i === run.pipeline!.cursor ? "当前步骤" : "待执行"}
                                        {run.pipeline!.batch?.steps[step.key]?.items.map((item, index) => (
                                            <div key={item.itemKey || index} className="ml-3 text-xs">
                                                {item.itemKey || step.key} · {item.status}
                                                {item.reusedFrom ? " · 已复用历史结果" : ""}
                                            </div>
                                        ))}
                                    </li>
                                ))}
                            </ol>
                            {["waiting_input", "running", "waiting_approval"].includes(run.status) &&
                                run.pipeline.inputs
                                    .filter((i) => i.status === "pending" && (!inputRequestId || i.id === inputRequestId))
                                    .map((input) => (
                                        <PluginInputForm
                                            key={`${scope}:${input.id}:${input.revision}`}
                                            run={run}
                                            input={input}
                                            onUpdated={(value) => {
                                                if (currentScope.current === scope) setLoaded({ scope, run: value });
                                            }}
                                        />
                                    ))}
                            {run.pipeline.childRunId && <PluginRunCard key={run.pipeline.childRunId} runId={run.pipeline.childRunId} canvasId={canvasId} />}
                            {run.pipeline.batch && run.pipeline.stepRuns?.filter((child) => !["succeeded", "cancelled", "failed"].includes(child.status)).map((child) => <PluginRunCard key={child.id} runId={child.id} canvasId={canvasId} />)}
                            {run.status === "paused" && (
                                <Button
                                    onClick={() => {
                                        void resumePluginRun(run, "retry_safe")
                                            .then((value) => {
                                                if (currentScope.current === scope) setLoaded({ scope, run: value });
                                            })
                                            .catch((e) => {
                                                if (currentScope.current === scope) setError(e.message);
                                            });
                                    }}
                                >
                                    重新检查并恢复流程
                                </Button>
                            )}
                        </>
                    )}
                    {run.executionAdapter === "http" && (
                        <PluginRemoteStatus
                            key={scope}
                            run={run}
                            onUpdated={(value) => {
                                if (currentScope.current === scope) setLoaded({ scope, run: value });
                            }}
                        />
                    )}
                    {((run.executionAdapter !== "http" && !run.pipeline) || run.status === "succeeded") && <PluginResultValues value={run.result ?? run.preview} view={run.view} />}
                    {canvasId &&
                        run.canvasActions?.map((action) => <PluginCanvasSave key={`${scope}:${action.operation}:${action.blueprintId}:${action.inputRequestId || ""}`} run={run} canvasId={canvasId} action={action} onCreated={(id) => setProjection({ scope, id })} />)}
                    {canvasId && run.executionAdapter === "canvas.blueprint.instantiate" && run.status === "succeeded" && (
                        <Button size="small" onClick={() => void refreshCanvasAfterAgent(canvasId).catch((cause) => setError(cause instanceof Error ? cause.message : "画布同步失败"))}>
                            同步画布结果
                        </Button>
                    )}
                </>
            )}
            {run?.status === "waiting_approval" && !run.pipeline && (
                <div className="flex flex-wrap gap-2">
                    <Button size="small" type="primary" loading={busy} onClick={() => void act("approve")}>
                        {run.executionAdapter === "http" ? "确认发送并执行" : run.executionAdapter === "canvas.blueprint.instantiate" ? "确认保存到画布" : "确认保存快照"}
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
            {run?.remote && ["running", "paused"].includes(run.status) && (
                <Button size="small" loading={busy} onClick={() => void act("cancel")}>
                    停止此任务
                </Button>
            )}
            {run?.pipeline && active && run.status !== "cancelling" && (
                <Button size="small" loading={busy} onClick={() => void act("cancel")}>
                    停止流程
                </Button>
            )}
        </section>
    );
}
