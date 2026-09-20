import { useEffect, useRef, useState } from "react";
import { Button, Select } from "antd";
import { Link, useNavigate, useSearchParams } from "react-router";
import { CanvasCloudAgentPanel, type CloudAgentPanelProps } from "./canvas-cloud-agent-panel";
import { appendAgentThreadReference, bindAgentThread, createAgentThread, getAgentThread, listAgentThreads, type AgentThread } from "@/services/api/agent-threads";
import { createCanvasProjectWithRemoteSync, saveRemoteUserDataNow } from "@/services/user-data-sync";
import { useUserStore } from "@/stores/use-user-store";
import { listPluginRuns } from "@/services/api/plugin-operations";
import { creationRuns } from "@/services/api/creation-runs";

type Props = Omit<CloudAgentPanelProps, "threadId" | "toolbar">;

export function AgentThreadWorkspace(props: Props) {
    const userId = useUserStore((state) => state.user?.id);
    return userId ? <OwnedThreadWorkspace key={userId} {...props} userId={userId} /> : null;
}

function OwnedThreadWorkspace({ userId, ...props }: Props & { userId: string }) {
    const [params, setParams] = useSearchParams();
    const navigate = useNavigate();
    const threadId = params.get("thread") || "";
    const [thread, setThread] = useState<AgentThread | null>(null);
    const [history, setHistory] = useState(false);
    const [threads, setThreads] = useState<AgentThread[]>([]);
    const [hasMore, setHasMore] = useState(false);
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState("");
    const [epoch, setEpoch] = useState(0);
    const [referenceKind, setReferenceKind] = useState<"plugin" | "creation">("plugin");
    const [referenceID, setReferenceID] = useState("");
    const [referenceOptions, setReferenceOptions] = useState<Array<{ value: string; label: string }>>([]);
    const [moreReferences, setMoreReferences] = useState(false);
    const lock = useRef(false);
    const createKey = useRef<string | null>(null);
    const canvasCreated = useRef<string | null>(null);
    const current = useRef(threadId);
    current.current = threadId;
    const owned = (id = threadId) => useUserStore.getState().user?.id === userId && current.current === id;

    useEffect(() => {
        let active = true;
        setThread(null); setError(""); setBusy(false);
        if (threadId) void getAgentThread(threadId).then((view) => { if (active) setThread(view.thread); }).catch((cause) => { if (active) setError(cause instanceof Error ? cause.message : String(cause)); });
        return () => { active = false; };
    }, [threadId, epoch]);

    useEffect(() => { canvasCreated.current = null; }, [threadId]);

    const selectThread = (id: string) => {
        const next = new URLSearchParams(params);
        if (id) next.set("thread", id); else next.delete("thread");
        next.set("agent", "1");setParams(next);setHistory(false);
    };
    const perform = async (work: () => Promise<void>) => {
        if (lock.current) return;
        lock.current = true; setBusy(true); setError("");
        try { await work(); } catch (cause) { if (owned()) setError(cause instanceof Error ? cause.message : String(cause)); }
        finally { lock.current = false; if (owned()) setBusy(false); }
    };
    const start = () => perform(async () => {
        createKey.current ||= crypto.randomUUID();
        if (props.canvasId) await saveRemoteUserDataNow();
        if (!owned()) return;
        const result = await createAgentThread(createKey.current, props.canvasId);
        if (!owned()) return;
        createKey.current = null; selectThread(result.thread.id);
    });
    const loadHistory = (more = false) => perform(async () => {
        const result = await listAgentThreads("", more ? threads.length : 0);
        if (!owned()) return;
        setThreads((rows) => more ? [...rows, ...result.threads.filter((row) => !rows.some((item) => item.id === row.id))] : result.threads);setHasMore(result.hasMore);setHistory(true);
    });
    const openCanvas = () => perform(async () => {
        const fresh = (await getAgentThread(threadId)).thread;
        if (!owned()) return;
        let canvasId = fresh.canvasId;
        if (!canvasId) {
            if (!canvasCreated.current) {
                const created = await createCanvasProjectWithRemoteSync(fresh.title);
                if (!owned()) return;
                if (!created.id) throw new Error("未能创建画布，请重试");
                canvasCreated.current = created.id;
                if (created.syncError) throw new Error("画布已保存在本机，云端尚未同步，请重试");
            } else await saveRemoteUserDataNow();
            if (!owned()) return;
            canvasId = canvasCreated.current;
            await bindAgentThread(threadId, fresh.revision, canvasId);
        }
        if (owned()) navigate(`/canvas/${encodeURIComponent(canvasId)}?agent=1&thread=${encodeURIComponent(threadId)}`);
    });
    const useCurrentCanvas = () => perform(async () => {
        await saveRemoteUserDataNow(); if (!owned()) return;
        const fresh = (await getAgentThread(threadId)).thread; if (!owned()) return;
        await bindAgentThread(threadId, fresh.revision, props.canvasId);
        if (owned()) setEpoch((value) => value + 1);
    });
    const attach = () => perform(async () => {
        const fresh = (await getAgentThread(threadId)).thread;if (!owned()) return;
        await appendAgentThreadReference(threadId, fresh.revision, referenceKind, referenceID.trim(), `reference-${referenceKind}-${referenceID.trim()}`);
        if (owned()) { setReferenceID("");setEpoch((value) => value + 1); }
    });
    const loadReferences = (more = false) => perform(async () => {
        const options = referenceKind === "plugin"
            ? (await listPluginRuns(more ? referenceOptions.length : 0)).map((run) => ({ value: run.id, label: `${run.operation} · ${run.status} · ${run.id.slice(-6)}` }))
            : (await creationRuns.list()).runs.map((run) => ({ value: run.id, label: `${typeof run.state.title === "string" ? run.state.title : "创作记录"} · ${new Date(run.createdAt).toLocaleString()} · ${run.status}` }));
        if (owned()) { setReferenceOptions((current) => more ? [...current, ...options.filter((item) => !current.some((saved) => saved.value === item.value))] : options);setMoreReferences(referenceKind === "plugin" && options.length === 30); }
    });
    const toolbar = <div className="border-b border-[var(--border)] px-4 py-2 text-xs" data-canvas-no-zoom data-canvas-wheel-scroll>
        <div className="flex flex-wrap items-center gap-2">
            <span>{thread?.title || (threadId ? "读取会话…" : props.embedded ? "Agent 对话" : "本机画布对话")}</span>
            <Button size="small" loading={busy} onClick={() => void start()}>新建云端会话</Button>
            <Button size="small" disabled={busy} onClick={() => void loadHistory()}>云端历史</Button>
            <Link to={props.canvasId || thread?.canvasId ? `/workbenches?canvasId=${encodeURIComponent(props.canvasId || thread?.canvasId || "")}` : "/workbenches"}>插件工作台</Link>
            {threadId ? <Button size="small" disabled={busy} onClick={() => setEpoch((value) => value + 1)}>重新读取</Button> : null}
            {threadId && props.embedded ? <Button size="small" disabled={busy || !thread} onClick={() => void openCanvas()}>打开画布</Button> : null}
            {threadId && !props.embedded ? <><Button size="small" onClick={() => navigate(`/create?agent=1&thread=${encodeURIComponent(threadId)}`)}>首页继续</Button><Button size="small" onClick={() => selectThread("")}>本机旧对话</Button></> : null}
        </div>
        {error ? <p role="alert" className="mt-2 text-[var(--status-error)]">{error}</p> : null}
        {thread && !props.embedded && thread.canvasId !== props.canvasId ? <p className="mt-2">此会话的下一轮尚未绑定当前画布。<Button size="small" disabled={busy} onClick={() => void useCurrentCanvas()}>使用当前画布继续</Button></p> : null}
        {history ? <div className="mt-2 max-h-56 overflow-auto">
            <div className="flex items-center justify-between"><span>全部云端会话</span><Button size="small" onClick={() => setHistory(false)}>收起</Button></div>
            {!threads.length ? <p>暂无云端会话。本机旧对话仍在原画布中。</p> : threads.map((item) => <Button block type="text" key={item.id} onClick={() => selectThread(item.id)}>{item.title} · {new Date(item.updatedAt).toLocaleDateString()}</Button>)}
            {hasMore ? <Button size="small" loading={busy} onClick={() => void loadHistory(true)}>更多会话</Button> : null}
            {threadId ? <div className="mt-2 flex flex-wrap gap-2"><Select size="small" value={referenceKind} disabled={busy} onChange={(kind) => { setReferenceKind(kind); setReferenceOptions([]); setReferenceID(""); setMoreReferences(false); }} options={[{ value: "plugin", label: "插件运行" }, { value: "creation", label: "创作运行" }]} /><Button size="small" disabled={busy} onClick={() => void loadReferences()}>选择已有记录</Button><Select size="small" className="min-w-48 max-w-full" aria-label="选择要关联的运行" value={referenceID || undefined} options={referenceOptions} onChange={setReferenceID} placeholder="选择已有记录" /><Button size="small" disabled={!referenceID || busy} onClick={() => void attach()}>关联记录</Button>{moreReferences ? <Button size="small" disabled={busy} onClick={() => void loadReferences(true)}>更多记录</Button> : null}</div> : null}
        </div> : null}
    </div>;

    if (!threadId && props.embedded) return <section className="w-full rounded-xl border border-[var(--border)] p-4">{toolbar}<p className="mt-4 text-sm text-[var(--muted-foreground)]">直接开始对话，按需引用技能和插件；需要创作时再打开画布。切换页面不会停止服务端运行。</p></section>;
    const canvasId = props.embedded ? thread?.canvasId || "" : props.canvasId;
    if (threadId && !thread) return props.open ? <div className={props.embedded ? "w-full" : "fixed bottom-6 right-6 z-[var(--z-modal-overlay)] max-w-lg rounded-xl bg-[var(--background)] p-3"}>{toolbar}</div> : <Button onClick={props.onOpen}>打开 Agent</Button>;
    return <CanvasCloudAgentPanel key={`${userId}:${threadId || "legacy"}:${epoch}`} {...props} canvasId={canvasId} threadId={threadId || undefined} toolbar={toolbar} onNewThread={() => void start()} onThreadHistory={() => void loadHistory()} />;
}
