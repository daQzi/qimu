import { Button, Checkbox, Input, InputNumber } from "antd";
import { useEffect, useRef, useState } from "react";
import type { ApplicationRelease } from "@/services/api/application-plugins";
import { getPluginConnections, getPluginPrice, savePluginConnection, savePluginPrice, type PluginConnection, type PluginOperationPrice } from "@/services/api/plugin-remote";
import { describePluginOperation, invokePluginOperation } from "@/services/api/plugin-operations";
import { useUserStore } from "@/stores/use-user-store";
import { PluginRunCard } from "./plugin-run-card";

export function PluginRemotePanel({ pluginId, release, admin }: { pluginId: string; release: ApplicationRelease; admin: boolean }) {
    const userID = useUserStore((s) => s.user?.id);
    return (
        <div className="space-y-3">
            {release.manifest.contributes.connectors?.map((connector) => (
                <ConnectionForm key={`${userID}:${pluginId}:${connector.id}`} pluginId={pluginId} connectorId={connector.id} />
            ))}
            {release.operations
                .filter((op) => op.execution.kind === "http")
                .map((op) => (
                    <div key={`${userID}:${release.id}:${op.id}`} className="space-y-2">
                        {admin && <PriceForm releaseId={release.id} operationId={op.id} />}
                        <RemoteOperationForm pluginId={pluginId} releaseId={release.id} operationId={op.id} description={op.description} />
                    </div>
                ))}
        </div>
    );
}
function ConnectionForm({ pluginId, connectorId }: { pluginId: string; connectorId: string }) {
    const userID = useUserStore((s) => s.user?.id);
    const [value, setValue] = useState<PluginConnection>({ pluginId, connectorId, name: connectorId, baseUrl: "", enabled: true, revision: 0 });
    const [credential, setCredential] = useState("");
    const [busy, setBusy] = useState(false);
    const [loaded, setLoaded] = useState(false);
    const [error, setError] = useState("");
    const current = () => useUserStore.getState().user?.id === userID;
    useEffect(() => {
        const controller = new AbortController();
        void getPluginConnections(controller.signal)
            .then((r) => {
                if (!current()) return;
                const existing = r.connections.find((c) => c.pluginId === pluginId && c.connectorId === connectorId);
                if (existing) setValue(existing);
                setLoaded(true);
            })
            .catch((e) => {
                if (!controller.signal.aborted && current()) setError(e.message);
            });
        return () => controller.abort();
    }, [userID, pluginId, connectorId]);
    const save = async () => {
        setBusy(true);
        setError("");
        try {
            const next = await savePluginConnection(value, credential);
            if (current()) {
                setValue(next);
                setCredential("");
            }
        } catch (e) {
            if (current()) setError(e instanceof Error ? e.message : "连接保存失败");
        } finally {
            if (current()) setBusy(false);
        }
    };
    return (
        <details className="space-y-2 text-sm">
            <summary className="cursor-pointer">远程连接：{connectorId}</summary>
            <p className="text-muted-foreground">先启用插件并授予连接权限。配置变更仅作用于新运行，在途任务使用批准时的版本。</p>
            <Input aria-label="连接名称" value={value.name} maxLength={120} onChange={(e) => setValue({ ...value, name: e.target.value })} />
            <Input aria-label="远程 API 地址" placeholder="https://供应商 API 基址" value={value.baseUrl} onChange={(e) => setValue({ ...value, baseUrl: e.target.value })} />
            <Input.Password
                aria-label="连接凭据"
                autoComplete="new-password"
                placeholder={value.credentialConfigured ? "已配置，留空保留；更换目标时需重新填写" : "API Key（无认证服务可留空）"}
                value={credential}
                onChange={(e) => setCredential(e.target.value)}
            />
            <Checkbox checked={value.enabled} onChange={(e) => setValue({ ...value, enabled: e.target.checked })}>
                启用此连接
            </Checkbox>
            <Button loading={busy} disabled={!loaded || !value.baseUrl} onClick={() => void save()}>
                保存连接
            </Button>
            {value.credentialConfigured && <p className="text-muted-foreground">凭据已加密保存，修订 {value.revision}</p>}
            {error && (
                <p role="alert" className="text-destructive">
                    {error}
                </p>
            )}
        </details>
    );
}
function PriceForm({ releaseId, operationId }: { releaseId: string; operationId: string }) {
    const userID = useUserStore((s) => s.user?.id);
    const [value, setValue] = useState<PluginOperationPrice>();
    const [error, setError] = useState("");
    const [busy, setBusy] = useState(false);
    useEffect(() => {
        const controller = new AbortController();
        void getPluginPrice(releaseId, operationId, controller.signal)
            .then((v) => {
                if (useUserStore.getState().user?.id === userID) setValue(v);
            })
            .catch((e) => {
                if (!controller.signal.aborted) setError(e.message);
            });
        return () => controller.abort();
    }, [releaseId, operationId, userID]);
    const save = async () => {
        if (!value) return;
        setBusy(true);
        setError("");
        try {
            const next = await savePluginPrice(value);
            if (useUserStore.getState().user?.id === userID) setValue(next);
        } catch (e) {
            setError(e instanceof Error ? e.message : "服务费保存失败");
        } finally {
            setBusy(false);
        }
    };
    return (
        <div className="flex flex-wrap items-center gap-2 text-sm">
            <span>{operationId} 平台服务费（积分/成功结果）</span>
            <InputNumber
                aria-label="平台服务费"
                min={0}
                max={1000}
                precision={6}
                value={value ? value.feeMicrocredits / 1_000_000 : null}
                onChange={(v) => {
                    if (value && typeof v === "number") setValue({ ...value, feeMicrocredits: Math.round(v * 1_000_000) });
                }}
            />
            <Button disabled={!value} loading={busy} onClick={() => void save()}>
                保存服务费
            </Button>
            {error && (
                <p role="alert" className="text-destructive">
                    {error}
                </p>
            )}
        </div>
    );
}
function RemoteOperationForm({ pluginId, releaseId, operationId, description }: { pluginId: string; releaseId: string; operationId: string; description: string }) {
    const userID = useUserStore((s) => s.user?.id);
    const [input, setInput] = useState("{}");
    const [runId, setRunId] = useState("");
    const [error, setError] = useState("");
    const [busy, setBusy] = useState(false);
    const attempt = useRef<{ signature: string; key: string } | undefined>(undefined);
    const invoke = async () => {
        setBusy(true);
        setError("");
        try {
            const values = JSON.parse(input);
            if (!values || Array.isArray(values) || typeof values !== "object") throw new Error("参数必须为 JSON 对象");
            await describePluginOperation(pluginId, operationId, releaseId);
            if (useUserStore.getState().user?.id !== userID) return;
            const signature = JSON.stringify(values);
            if (attempt.current?.signature !== signature) attempt.current = { signature, key: crypto.randomUUID() };
            const result = await invokePluginOperation({ operation: `${pluginId}.${operationId}`, releaseId, input: values }, attempt.current.key);
            if (useUserStore.getState().user?.id !== userID) return;
            if (result.kind !== "run") throw new Error("远程操作未返回持久运行");
            setRunId(result.runId);
            attempt.current = undefined;
        } catch (e) {
            setError(e instanceof Error ? e.message : "提交失败，同一参数可重试");
        } finally {
            setBusy(false);
        }
    };
    return (
        <details className="space-y-2 text-sm">
            <summary className="cursor-pointer">调试远程操作：{operationId}</summary>
            <p>{description}</p>
            <Input.TextArea aria-label="远程操作参数 JSON" rows={3} value={input} onChange={(e) => setInput(e.target.value)} />
            <Button loading={busy} onClick={() => void invoke()}>
                创建待确认运行
            </Button>
            {error && (
                <p role="alert" className="text-destructive">
                    {error}
                </p>
            )}
            {runId && <PluginRunCard runId={runId} />}
        </details>
    );
}
