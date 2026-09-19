import { Button, Input } from "antd";
import { useRef, useState } from "react";
import { useUserStore } from "@/stores/use-user-store";
import { createApplicationPluginHost } from "@/services/plugin-host";
import type { ApplicationRelease } from "@/services/api/application-plugins";
import { PluginRunCard } from "./plugin-run-card";

export function PluginOperationTester({ pluginId, release }: { pluginId: string; release: ApplicationRelease }) {
    const userID = useUserStore((s) => s.user?.id);
    const [resourceId, setResourceId] = useState("");
    const [runInput, setRunInput] = useState("");
    const [runId, setRunId] = useState("");
    const [result, setResult] = useState<unknown>();
    const [error, setError] = useState("");
    const [busy, setBusy] = useState(false);
    const attempt = useRef<{ signature: string; key: string } | undefined>(undefined);
    const host = createApplicationPluginHost(pluginId, release.id);
    const execute = async (operationId: string) => {
        if (busy || !resourceId.trim()) return;
        setBusy(true);
        setError("");
        const signature = `${userID}:${release.id}:${operationId}:${resourceId.trim()}`;
        if (attempt.current?.signature !== signature) attempt.current = { signature, key: crypto.randomUUID() };
        try {
            await host.describe(operationId);
            if (useUserStore.getState().user?.id !== userID) return;
            const response = await host.invoke(operationId, { resourceId: resourceId.trim() }, attempt.current.key);
            if (useUserStore.getState().user?.id !== userID) return;
            if (response.kind === "inline") setResult(response.result);
            else {
                setRunId(response.runId);
                setRunInput(response.runId);
            }
            attempt.current = undefined;
        } catch (cause) {
            if (useUserStore.getState().user?.id === userID) setError(cause instanceof Error ? cause.message : "调用失败，可用相同输入重试");
        } finally {
            setBusy(false);
        }
    };
    return (
        <details className="space-y-2 text-sm">
            <summary className="cursor-pointer">操作调试与运行查询</summary>
            <Input aria-label="视频资源 ID" placeholder="填写当前账号已上传视频的资源 ID" maxLength={80} value={resourceId} onChange={(event) => setResourceId(event.target.value)} />
            <div className="flex flex-wrap gap-2">
                {release.operations.map((op) => (
                    <Button key={op.id} disabled={!op.available || busy || !resourceId.trim()} onClick={() => void execute(op.id)}>
                        {op.effects.includes("draft_write") ? "保存元数据快照" : "读取视频信息"}
                    </Button>
                ))}
            </div>
            {error && (
                <p role="alert" className="text-destructive">
                    {error}
                </p>
            )}
            {result !== undefined && <pre className="max-h-48 overflow-auto whitespace-pre-wrap break-all text-xs">{JSON.stringify(result, null, 2)}</pre>}
            <div className="flex gap-2">
                <Input aria-label="插件运行 ID" placeholder="运行 ID，可用于刷新后恢复查看" value={runInput} onChange={(event) => setRunInput(event.target.value)} />
                <Button onClick={() => setRunId(runInput.trim())} disabled={!runInput.trim()}>
                    查询运行
                </Button>
            </div>
            {runId && <PluginRunCard key={runId} runId={runId} />}
        </details>
    );
}
