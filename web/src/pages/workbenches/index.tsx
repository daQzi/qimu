import { useEffect, useRef, useState } from "react";
import { Button, Input, Select } from "antd";
import { Link, useParams, useSearchParams } from "react-router";
import { useUserStore } from "@/stores/use-user-store";
import { getWorkbench, listWorkbenches, previewWorkbench, type WorkbenchPreview, type WorkbenchView } from "@/services/api/plugin-workbenches";
import { loadWorkbenchDraft, saveWorkbenchDraft, type WorkbenchDraft } from "@/services/workbench-drafts";
import { invokePluginOperation } from "@/services/api/plugin-operations";
import { WorkbenchFields } from "@/components/plugins/workbench-fields";
import { PluginRunCard } from "@/components/plugins/plugin-run-card";
import { PluginResultValues } from "@/components/plugins/plugin-result-values";
import { AgentThreadWorkspace } from "@/components/canvas/agent-thread-workspace";
import type { PluginInvocationContext } from "@/lib/plugins/plugin-v3-types";

export default function WorkbenchesPage() {
    const user = useUserStore((s) => s.user?.id);
    const { id = "" } = useParams();
    const [params] = useSearchParams();
    const canvas = params.get("canvasId") || "";
    const release = params.get("releaseId") || "";
    return user ? <OwnedWorkbench key={`${user}:${id}:${release}:${canvas}`} user={user} id={id} release={release} canvas={canvas} /> : null;
}

function OwnedWorkbench({ user, id, release, canvas }: { user: string; id: string; release: string; canvas: string }) {
    const [view, setView] = useState<WorkbenchView>();
    const [catalog, setCatalog] = useState<WorkbenchView[]>([]);
    const [draft, setDraft] = useState<WorkbenchDraft>({ input: {}, recipes: [], goal: "" });
    const [preview, setPreview] = useState<WorkbenchPreview>();
    const [result, setResult] = useState<unknown>();
    const [ready, setReady] = useState(false);
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState("");
    const [notice, setNotice] = useState("");
    const [agentOpen, setAgentOpen] = useState(false);
    const [fieldsValid, setFieldsValid] = useState(true);
    const active = useRef(true);
    const lock = useRef(false);
    const saves = useRef<Promise<unknown>>(Promise.resolve());
    const context: PluginInvocationContext = canvas ? { hostSurface: "canvas", canvasId: canvas } : { hostSurface: "agent-home" };
    const owned = () => active.current && useUserStore.getState().user?.id === user;
    useEffect(() => {
        active.current = true;
        void (async () => {
            if (!id) {
                const rows = await listWorkbenches();
                if (owned()) {
                    setCatalog(rows);
                    setReady(true);
                }
                return;
            }
            const board = await getWorkbench(id, release, context);
            const saved = await loadWorkbenchDraft(user, id, board.releaseId, canvas);
            if (owned()) {
                setView(board);
                if (saved) setDraft(saved);
                setReady(true);
            }
        })().catch((cause) => {
            if (owned()) setError(String(cause instanceof Error ? cause.message : cause));
        });
        return () => {
            active.current = false;
        };
    }, []);
    const perform = async (work: () => Promise<void>) => {
        if (lock.current) return;
        lock.current = true;
        setBusy(true);
        setError("");
        setNotice("");
        try {
            await work();
        } catch (cause) {
            if (owned()) setError(cause instanceof Error ? cause.message : String(cause));
        } finally {
            lock.current = false;
            if (owned()) setBusy(false);
        }
    };
    const persist = async (next: WorkbenchDraft) => {
        if (!view || !owned()) throw new Error("工作台上下文已变化");
        const save = saves.current.catch(() => undefined).then(() => saveWorkbenchDraft(user, id, view.releaseId, canvas, next));
        saves.current = save;
        await save;
        if (owned()) setDraft(next);
    };
    const edit = (next: WorkbenchDraft) => {
        setDraft(next);
        setPreview(undefined);
        if (!view) return;
        const save = saves.current.catch(() => undefined).then(() => saveWorkbenchDraft(user, id, view.releaseId, canvas, next));
        saves.current = save;
        void save.catch((cause) => {
            if (owned()) setError(`本机草稿未保存：${String(cause)}`);
        });
    };
    const compose = () =>
        perform(async () => {
            if (!view || !fieldsValid) return;
            await persist(draft);
            if (!owned()) return;
            const value = await previewWorkbench(id, view.releaseId, draft.recipes, draft.input, context);
            if (owned()) setPreview(value);
        });
    const run = () =>
        perform(async () => {
            if (!view) return;
            let pending = draft.pending;
            if (!pending) {
                if (!preview?.valid || !view.definition.operation) throw new Error("请先预览并确认输入");
                pending = { key: crypto.randomUUID(), request: { operation: view.definition.operation, releaseId: view.releaseId, input: preview.selection.input, context, workbench: preview.selection } };
                // Persist the exact request before POST; an ambiguous response only retries this identity.
                await persist({ ...draft, pending });
            }
            if (!owned()) return;
            const receipt = await invokePluginOperation(pending.request, pending.key);
            if (!owned()) return;
            await persist({ ...draft, pending: undefined, runId: receipt.kind === "run" ? receipt.runId : undefined });
            if (!owned()) return;
            if (receipt.kind === "inline") setResult(receipt.result);
            setNotice(receipt.kind === "inline" ? "检查完成" : "已创建运行，请在下方继续确认或查看结果");
        });
    const launchAgent = () =>
        perform(async () => {
            if (!preview?.valid) throw new Error("请先预览并确认输入");
            await persist({ ...draft, agent: preview.selection, agentGoal: draft.goal });
            if (owned()) setAgentOpen(true);
        });
    const locked = busy || !!draft.pending;
    return (
        <main className="mx-auto w-full max-w-5xl space-y-5 p-4 md:p-6">
            <div className="flex flex-wrap items-center gap-4">
                <h1 className="text-xl font-semibold">{view?.definition.name || "插件工作台"}</h1>
                <Link to="/plugins">应用插件</Link>
                <Link to="/business-objects">品牌资料</Link>
                {id ? <Link to={canvas ? `/workbenches?canvasId=${encodeURIComponent(canvas)}` : "/workbenches"}>切换工作台</Link> : null}
            </div>
            <p className="text-sm text-muted-foreground">由插件提供流程和输入建议。模型、费用及写入授权由你确认。切换工作台前请保存草稿，切换不会停止已提交任务。</p>
            {error ? (
                <p role="alert" className="text-destructive">
                    {error}
                </p>
            ) : null}
            {notice ? <p role="status">{notice}</p> : null}
            {!ready && !error ? <p>正在读取工作台…</p> : null}
            {!id && ready ? (
                <div className="grid gap-4 md:grid-cols-2">
                    {!catalog.length ? (
                        <p>暂无可用工作台，请先在应用插件中启用支持工作台的插件。</p>
                    ) : (
                        catalog.map((row) => (
                            <article key={row.id} className="space-y-2 rounded-lg border border-border bg-card p-4">
                                <h2>{row.definition.name}</h2>
                                <p className="text-sm text-muted-foreground">{row.definition.description}</p>
                                <p className="text-xs">版本 {row.version}</p>
                                {row.definition.hostSurfaces.includes(canvas ? "canvas" : "agent-home") ? (
                                    <Link to={`/workbenches/${encodeURIComponent(row.id)}?${new URLSearchParams({ releaseId: row.releaseId, ...(canvas ? { canvasId: canvas } : {}) })}`}>打开工作台</Link>
                                ) : (
                                    <p>请从支持的画布入口打开</p>
                                )}
                            </article>
                        ))
                    )}
                </div>
            ) : null}
            {view && ready ? (
                <>
                    <section className="space-y-4 rounded-lg border border-border bg-card p-4">
                        <p>{view.definition.description}</p>
                        <p className="text-xs text-muted-foreground">
                            版本 {view.version} · {canvas ? "当前画布" : "首页 Agent"}
                        </p>
                        <label className="block space-y-1">
                            <span>配方（可多选）</span>
                            <Select
                                className="w-full"
                                aria-label="配方"
                                mode="multiple"
                                value={draft.recipes}
                                disabled={locked}
                                options={Object.values(view.recipes).map((recipe) => ({ value: recipe.id, label: recipe.name }))}
                                onChange={(recipes) => edit({ ...draft, recipes })}
                            />
                        </label>
                        <WorkbenchFields
                            schema={view.schema}
                            objectInputs={view.definition.objectInputs}
                            value={draft.input}
                            suggested={preview?.input || view.definition.defaults}
                            disabled={locked}
                            onChange={(input) => edit({ ...draft, input })}
                            onValidityChange={setFieldsValid}
                        />
                        <label className="block space-y-1">
                            <span>给 Agent 的目标（可选）</span>
                            <Input.TextArea aria-label="给 Agent 的目标" value={draft.goal} disabled={locked} onChange={(e) => edit({ ...draft, goal: e.target.value })} />
                        </label>
                        <div className="flex flex-wrap gap-2">
                            <Button
                                disabled={locked}
                                onClick={() =>
                                    void perform(async () => {
                                        await persist(draft);
                                        if (owned()) setNotice("草稿已保存在当前账号的本机缓存");
                                    })
                                }
                            >
                                保存本机草稿
                            </Button>
                            <Button disabled={locked} onClick={() => void compose()}>
                                预览最终输入
                            </Button>
                            {view.definition.operation ? (
                                <Button type="primary" disabled={busy || (!draft.pending && !preview?.valid)} onClick={() => void run()}>
                                    {draft.pending ? "原样重试待确认请求" : "确认执行操作"}
                                </Button>
                            ) : null}
                            <Button disabled={locked || !preview?.valid} onClick={() => void launchAgent()}>
                                交给 Agent
                            </Button>
                        </div>
                        {draft.pending ? <p role="status">上次请求结果待确认。输入已锁定，原样重试会使用同一个请求编号；也可去插件运行历史核对。</p> : null}
                        {preview ? (
                            <div className="space-y-2">
                                <h2>最终输入</h2>
                                <PluginResultValues value={preview.input} />
                                {preview.conflicts.map((c, i) => (
                                    <p key={i} role="alert">
                                        {c.field}：{c.message}
                                    </p>
                                ))}
                                {preview.validationMessage ? <p role="alert">{preview.validationMessage}</p> : null}
                                {Object.entries(preview.objects || {}).map(([field, object]) => (
                                    <details key={field}>
                                        <summary>
                                            {object.brand.name} · 固定版本 {object.reference.version}
                                        </summary>
                                        <PluginResultValues value={object.brand} />
                                    </details>
                                ))}
                                {preview.prompts.map((text, i) => (
                                    <p key={i} className="whitespace-pre-wrap text-sm">
                                        {text}
                                    </p>
                                ))}
                            </div>
                        ) : null}
                    </section>
                    {result !== undefined ? <PluginResultValues value={result} /> : null}
                    {draft.runId ? <PluginRunCard runId={draft.runId} canvasId={canvas || undefined} expectedReleaseId={view.releaseId} /> : null}
                    {(agentOpen || draft.agent) && draft.agent ? (
                        <section className="space-y-3">
                            <h2>Agent 协作</h2>
                            <p className="text-sm text-muted-foreground">已固定交给 Agent 的输入。下面新建会话，选择模型并发送目标；修改表单不会改变正在执行的任务。</p>
                            <AgentThreadWorkspace
                                canvasId={canvas}
                                references={[]}
                                nodeCount={0}
                                open
                                embedded
                                workbench={draft.agent}
                                prefillPrompt={draft.agentGoal || `请根据工作台“${view.definition.name}”的输入完成任务。`}
                                onOpen={() => setAgentOpen(true)}
                                onCollapse={() => setAgentOpen(false)}
                            />
                        </section>
                    ) : null}
                </>
            ) : null}
        </main>
    );
}
