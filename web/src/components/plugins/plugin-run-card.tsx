import { Button } from "antd";
import { useEffect, useRef, useState } from "react";
import { useUserStore } from "@/stores/use-user-store";
import { cancelPluginRun, decidePluginRun, getPluginRun, type PluginRunView } from "@/services/api/plugin-operations";

export function PluginRunCard({ runId }: { runId: string }) {
    const userID = useUserStore((s) => s.user?.id);
    const scope = `${userID || ""}:${runId}`;
    const currentScope = useRef(scope);
    currentScope.current = scope;
    const [loaded, setLoaded] = useState<{ scope: string; run: PluginRunView }>();
    const [error, setError] = useState("");
    const [busy, setBusy] = useState(false);
    const [refresh, setRefresh] = useState(0);
    const run = loaded?.scope === scope ? loaded.run : undefined;
    useEffect(() => {
        const controller = new AbortController();
        setError("");
        setBusy(false);
        void getPluginRun(runId, controller.signal)
            .then((value) => {
                if (currentScope.current === scope) setLoaded({ scope, run: value });
            })
            .catch((cause) => {
                if (!controller.signal.aborted && currentScope.current === scope) setError(cause instanceof Error ? cause.message : "读取运行失败");
            });
        return () => controller.abort();
    }, [scope, runId, refresh]);
    const act = async (action: "approve" | "reject" | "cancel") => {
        if (!run || busy) return;
        setBusy(true);
        setError("");
        try {
            const value = action === "cancel" ? await cancelPluginRun(run) : await decidePluginRun(run, action);
            if (currentScope.current === scope) setLoaded({ scope, run: value });
        } catch (cause) {
            if (currentScope.current === scope) setError(cause instanceof Error ? cause.message : "操作失败，请重新读取状态");
        } finally {
            if (currentScope.current === scope) setBusy(false);
        }
    };
    return (
        <section aria-label="插件运行" className="m-2 space-y-2 rounded-lg border border-border bg-card p-3 text-sm">
            <div className="flex items-center justify-between gap-2">
                <span>插件运行：{run?.status === "waiting_approval" ? "等待确认" : run?.status === "succeeded" ? "已完成" : run?.status === "cancelled" ? "已取消" : "读取中"}</span>
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
                    <p>{run.operation}</p>
                    <details>
                        <summary className="cursor-pointer">{run.status === "succeeded" ? "查看保存结果" : "查看待保存内容"}</summary>
                        <pre className="max-h-48 overflow-auto whitespace-pre-wrap break-all text-xs">{JSON.stringify(run.result ?? run.preview, null, 2)}</pre>
                    </details>
                </>
            )}
            {run?.status === "waiting_approval" && (
                <div className="flex flex-wrap gap-2">
                    <Button size="small" type="primary" loading={busy} onClick={() => void act("approve")}>
                        确认保存快照
                    </Button>
                    <Button size="small" disabled={busy} onClick={() => void act("reject")}>
                        拒绝
                    </Button>
                    <Button size="small" disabled={busy} onClick={() => void act("cancel")}>
                        取消运行
                    </Button>
                </div>
            )}
        </section>
    );
}
