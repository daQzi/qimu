import { Button, Input } from "antd";
import { useState } from "react";
import type { PluginRunView } from "@/services/api/plugin-operations";
import { resumePluginRun } from "@/services/api/plugin-remote";
import { useUserStore } from "@/stores/use-user-store";

export function PluginRemoteStatus({ run, onUpdated }: { run: PluginRunView; onUpdated: (value: PluginRunView) => void }) {
    const userID = useUserStore((s) => s.user?.id);
    const [jobID, setJobID] = useState("");
    const [error, setError] = useState("");
    const [busy, setBusy] = useState(false);
    const preview = (run.preview || {}) as { targetHost?: string; platformFeeMicrocredits?: number; resourceCount?: number; idempotency?: string; asynchronous?: boolean; lookupSupported?: boolean };
    const resume = async (action: "retry_safe" | "retry_import" | "retry_poll" | "attach_job") => {
        setBusy(true);
        setError("");
        try {
            const updated = await resumePluginRun(run, action, action === "attach_job" ? jobID.trim() : undefined);
            if (useUserStore.getState().user?.id === userID) onUpdated(updated);
        } catch (e) {
            if (useUserStore.getState().user?.id === userID) setError(e instanceof Error ? e.message : "恢复失败");
        } finally {
            setBusy(false);
        }
    };
    return (
        <div className="space-y-2 text-sm">
            {run.status === "waiting_approval" && (
                <>
                    <p>
                        将发送到 {preview.targetHost}，涉及 {preview.resourceCount ?? 0} 个素材。
                    </p>
                    <p>平台服务费：{typeof preview.platformFeeMicrocredits === "number" ? preview.platformFeeMicrocredits / 1_000_000 : "待确认"} 积分（成功保存结果时结算）。供应商使用你的 Key 单独计费，其费用不包含在平台积分中。</p>
                </>
            )}
            {run.remote && (
                <p className="text-muted-foreground">
                    {(
                        {
                            prepared: "等待后台提交",
                            submitting: "提交中",
                            submitted: "已提交，等待查询",
                            polling: "正在查询原任务",
                            unknown: "提交结果待核实",
                            upstream_completed: "上游已完成，等待导入",
                            import_pending: "结果导入待恢复",
                            imported: "结果已保存",
                            cancelled: "本地任务已停止",
                            failed: "执行失败",
                        } as Record<string, string>
                    )[run.remote.submissionState] || run.remote.submissionState}
                </p>
            )}
            {run.remote?.failureMessage && run.remote.failureMessage !== run.failureMessage && <p className="text-destructive">{run.remote.failureMessage}</p>}
            {run.remote?.providerJobId && <p className="break-all text-xs">上游任务：{run.remote.providerJobId}</p>}
            {run.status === "paused" && (
                <div className="space-y-2">
                    {run.remote?.submissionState === "import_pending" ? (
                        <Button loading={busy} onClick={() => void resume("retry_import")}>
                            重试导入已有结果
                        </Button>
                    ) : run.remote?.providerJobId ? (
                        <Button loading={busy} onClick={() => void resume("retry_poll")}>
                            恢复原任务查询
                        </Button>
                    ) : preview.idempotency === "header" || preview.lookupSupported ? (
                        <Button loading={busy} onClick={() => void resume("retry_safe")}>
                            按原提交身份恢复
                        </Button>
                    ) : (
                        <p>该上游未声明安全重试能力，请先在供应商侧核实；不要直接重复生成。</p>
                    )}
                    {run.remote?.submissionState === "unknown" && preview.asynchronous && (
                        <>
                            <Input aria-label="核实后的上游任务 ID" placeholder="从供应商取得的真实任务 ID" value={jobID} maxLength={160} onChange={(e) => setJobID(e.target.value)} />
                            <Button disabled={!jobID.trim()} loading={busy} onClick={() => void resume("attach_job")}>
                                绑定该任务并继续查询
                            </Button>
                        </>
                    )}
                </div>
            )}
            {error && (
                <p role="alert" className="text-destructive">
                    {error}
                </p>
            )}
        </div>
    );
}
