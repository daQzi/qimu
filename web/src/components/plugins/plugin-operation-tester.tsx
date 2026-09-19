import { Button, Input } from "antd";
import { useRef, useState } from "react";
import { useUserStore } from "@/stores/use-user-store";
import { createApplicationPluginHost } from "@/services/plugin-host";
import type { ApplicationRelease } from "@/services/api/application-plugins";
import { PluginRunCard } from "./plugin-run-card";
import { PluginResultValues } from "./plugin-result-values";
import type { PluginResultView } from "@/services/api/plugin-operations";

export function PluginOperationTester({ pluginId, release }: { pluginId: string; release: ApplicationRelease }) {
    const userID = useUserStore((s) => s.user?.id);
    const [resourceId, setResourceId] = useState("");
    const [runInput, setRunInput] = useState("");
    const [runId, setRunId] = useState("");
    const [result, setResult] = useState<{ resourceId: string; digest: string; value: unknown; view?: PluginResultView; userID?: string }>();
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
            const description = await host.describe(operationId);
            if (useUserStore.getState().user?.id !== userID) return;
            const schema = description.schemas[description.definition.inputSchemaRef] as { properties?: Record<string, unknown> } | undefined;
            const input: Record<string, unknown> = { resourceId: resourceId.trim() };
            if (schema?.properties?.expectedDigest) {
                if (!result || result.resourceId !== resourceId.trim() || result.userID !== userID) throw new Error("请先读取此视频信息，再保存已检查的结果");
                input.expectedDigest = result.digest;
            }
            const response = await host.invoke(operationId, input, attempt.current.key);
            if (useUserStore.getState().user?.id !== userID) return;
            if (response.kind === "inline") setResult({ resourceId: resourceId.trim(), value: response.result, digest: response.digest, view: description.resultView, userID });
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
                {release.operations.filter((op) => op.execution.kind === "host" && ["resource.inspect", "resource.snapshot"].includes(op.execution.adapter)).map((op) => (
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
            {result !== undefined && result.userID === userID && <PluginResultValues value={result.value} view={result.view} />}
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
