import { Button, Checkbox, Input, Select } from "antd";
import { useRef, useState } from "react";
import { approvePluginBatch, derivePluginRun, getPluginBatchQuote, type PluginBatchQuote, type PluginRunView } from "@/services/api/plugin-operations";

export function PluginBatchControls({ run, onUpdated, onDerived }: { run: PluginRunView; onUpdated: (run: PluginRunView) => void; onDerived: (id: string) => void }) {
    const [quote, setQuote] = useState<PluginBatchQuote>();
    const [external, setExternal] = useState(false);
    const [reuse, setReuse] = useState(false);
    const [edit, setEdit] = useState(false);
    const [inputs, setInputs] = useState("{}");
    const [forceSteps, setForceSteps] = useState<string[]>([]);
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState("");
    const deriveKey = useRef<{ body: string; key: string } | undefined>(undefined);
    const execute = async (action: () => Promise<void>) => {
        if (busy) return;
        setBusy(true);
        setError("");
        try {
            await action();
        } catch (e) {
            setError(e instanceof Error ? e.message : "操作失败");
        } finally {
            setBusy(false);
        }
    };
    return (
        <div className="space-y-2">
            {run.derivedFromRunId && (
                <p>
                    派生自 {run.derivedFromRunId} · 第 {run.attempt} 次运行
                </p>
            )}
            {error && (
                <p role="alert" className="text-destructive">
                    {error}
                </p>
            )}
            {run.status === "waiting_approval" && (
                <Button
                    size="small"
                    disabled={busy}
                    onClick={() =>
                        void execute(async () => {
                            setQuote(await getPluginBatchQuote(run.id));
                            setExternal(false);
                        })
                    }
                >
                    核对当前批次费用
                </Button>
            )}
            {quote && quote.items.length > 0 && (
                <div className="space-y-2">
                    <p>
                        本次批准 {quote.items.length} 项远程操作，平台服务费共 {quote.amountMicrocredits} 微积分。授权有效至 {new Date(quote.expiresAt).toLocaleTimeString()}。
                    </p>
                    <ul className="list-inside list-disc">
                        {quote.items.map((item) => (
                            <li key={item.runId}>
                                {item.operation} · {item.feeMicrocredits} 微积分 · {item.runId}
                            </li>
                        ))}
                    </ul>
                    {!quote.vendorCostKnown && (
                        <Checkbox checked={external} onChange={(e) => setExternal(e.target.checked)}>
                            我接受供应商独立计费：金额未知，不计入上面的平台服务费额度。
                        </Checkbox>
                    )}
                    <Button
                        size="small"
                        disabled={busy || (!quote.vendorCostKnown && !external)}
                        onClick={() =>
                            void execute(async () => {
                                onUpdated(await approvePluginBatch(run.id, quote, external));
                                setQuote(undefined);
                                window.dispatchEvent(new CustomEvent("wallet:updated"));
                            })
                        }
                    >
                        批准此批次并预留服务费
                    </Button>
                </div>
            )}
            {quote?.items.length === 0 && <p>当前没有可批量批准的远程任务；系统模型、画布和草稿操作仍需分别确认。</p>}
            {["succeeded", "failed", "cancelled"].includes(run.status) && (
                <>
                    <Button
                        size="small"
                        onClick={() => {
                            setEdit(!edit);
                            if (!edit) setInputs(JSON.stringify(Object.fromEntries((run.pipeline?.inputs || []).filter((i) => i.status === "submitted").map((i) => [i.stepKey, i.submitted])), null, 2));
                        }}
                    >
                        基于此次运行重新执行
                    </Button>
                    {edit && (
                        <div className="space-y-2">
                            <p>按步骤名称修改已提交的表单内容。会创建新运行，保留历史结果；新远程任务仍需批准。</p>
                            <Input.TextArea aria-label="派生表单输入 JSON" value={inputs} onChange={(e) => setInputs(e.target.value)} autoSize={{ minRows: 3, maxRows: 12 }} />
                            <Select
                                className="w-full"
                                mode="multiple"
                                aria-label="强制重做步骤"
                                placeholder="选择必须重做的步骤（包括依赖它的后续步骤）"
                                options={run.pipeline?.steps.map((s) => ({ value: s.key, label: s.key }))}
                                value={forceSteps}
                                onChange={setForceSteps}
                            />
                            <Checkbox checked={reuse} onChange={(e) => setReuse(e.target.checked)}>
                                明确复用输入和资源未变化的成功远程结果（不会重新随机生成）
                            </Checkbox>
                            <Button
                                size="small"
                                disabled={busy}
                                onClick={() =>
                                    void execute(async () => {
                                        const parsed = JSON.parse(inputs);
                                        if (!parsed || Array.isArray(parsed) || typeof parsed !== "object") throw new Error("请输入以步骤名称为键的 JSON 对象");
                                        const request = { inputs: parsed, forceSteps, reuseCompleted: reuse };
                                        const body = JSON.stringify(request);
                                        if (deriveKey.current?.body !== body) deriveKey.current = { body, key: crypto.randomUUID() };
                                        const next = await derivePluginRun(run.id, request, deriveKey.current.key);
                                        onDerived(next.id);
                                        setEdit(false);
                                    })
                                }
                            >
                                创建派生运行
                            </Button>
                        </div>
                    )}
                </>
            )}
        </div>
    );
}
