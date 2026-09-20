import { Button, Checkbox, Input, InputNumber, Select } from "antd";
import { useEffect, useRef, useState } from "react";
import { resourceFileUrl, getResource, uploadResourceFile, listResourceMaterials, getResourceFrame, type RemoteResource } from "@/services/api/resources";
import { mergeReportCharacters, type VideoReport, type VideoPlan } from "@/lib/plugins/media-review";

type Props = { value: Record<string, unknown>; mode: "report" | "plan"; disabled: boolean; onChange: (value: Record<string, unknown>) => void; onBusyChange: (busy: boolean) => void };
export function PluginMediaEditor({ value, mode, disabled, onChange, onBusyChange }: Props) {
    const report = value.report as VideoReport | undefined;
    const plan = value.plan as VideoPlan | undefined;
    const video = useRef<HTMLVideoElement>(null);
    const [error, setError] = useState("");
    const [uploading, setUploading] = useState(false);
    const [materials, setMaterials] = useState<RemoteResource[]>([]);
    const [frame, setFrame] = useState<{ url: string; atMs: number }>();
    const frameRequest = useRef<AbortController | undefined>(undefined);
    useEffect(() => { onBusyChange(uploading); return () => onBusyChange(false); }, [uploading, onBusyChange]);
    useEffect(() => {
        const controller = new AbortController();
        if (mode === "plan")
            void listResourceMaterials(controller.signal)
                .then((v) => setMaterials(v.resources.filter((r) => r.status === "ready")))
                .catch((e) => {
                    if (!controller.signal.aborted) setError(e instanceof Error ? e.message : "读取素材失败");
                });
        return () => controller.abort();
    }, [mode]);
    useEffect(
        () => () => {
            if (frame) URL.revokeObjectURL(frame.url);
        },
        [frame],
    );
    useEffect(() => () => frameRequest.current?.abort(), []);
    const busy = disabled || uploading;
    const update = (report: VideoReport) => onChange({ ...value, report });
    if (!report?.source || !report.coverage || !Array.isArray(report.entities) || !report.entities.every((e)=>e && Array.isArray(e.evidence) && e.evidence.every(Boolean) && Array.isArray(e.uncertainties)) || !Array.isArray(report.shots) || !report.shots.every(Boolean) || !Array.isArray(report.dialogue) || !report.dialogue.every(Boolean) || !Array.isArray(report.limitations) || (report.audioEvents !== undefined && (!Array.isArray(report.audioEvents) || !report.audioEvents.every(Boolean)))) return <p role="alert">报告结构不完整，请切换 JSON 编辑修正。</p>;
    const seek = (atMs: number) => {
        if (video.current) video.current.currentTime = atMs / 1000;
    };
    const characters = report.entities.filter((e) => e.kind === "character");
    const options = characters.map((e) => ({ value: e.id, label: e.name }));
    const extract = async () => {
        frameRequest.current?.abort();
        const controller = new AbortController();
        frameRequest.current = controller;
        const atMs = Math.max(0, Math.min(report.source.durationMs - 1, Math.round((video.current?.currentTime || 0) * 1000)));
        try {
            const response = await getResourceFrame(report.source.resourceId, atMs, controller.signal);
            if (!controller.signal.aborted) setFrame({ url: URL.createObjectURL(response.data), atMs });
        } catch (e) {
            if (!controller.signal.aborted) setError(e instanceof Error ? e.message : "抽帧失败");
        }
    };
    const replaceMapping = (index: number, patch: Partial<VideoPlan["mappings"][number]>) => {
        if (plan) onChange({ ...value, plan: { ...plan, mappings: plan.mappings.map((m, i) => (i === index ? { ...m, ...patch } : m)) } });
    };
    const material = async (index: number, file: File) => {
        if (!plan || busy) return;
        const kind = plan.mappings[index].kind === "voice" ? "audio" : file.type.startsWith("video/") ? "video" : "image";
        setUploading(true);
        setError("");
        try {
            if (!file.type.startsWith(kind + "/")) throw new Error("素材类型不符合替换项要求");
            const uploaded = await uploadResourceFile(file, kind);
            replaceMapping(index, { materialId: uploaded.id });
        } catch (cause) {
            setError(cause instanceof Error ? cause.message : "素材上传失败");
        } finally {
            setUploading(false);
        }
    };
    return (
        <div className="space-y-3" data-canvas-no-zoom data-canvas-wheel-scroll>
            <video ref={video} src={resourceFileUrl(report.source.resourceId)} controls preload="metadata" className="max-h-64 w-full" />
            <Button size="small" disabled={busy} onClick={() => void extract()}>
                提取当前时间画面
            </Button>
            {frame && (
                <figure>
                    <img src={frame.url} alt={`原片 ${frame.atMs} ms 画面`} className="max-h-48" />
                    <figcaption>请求时间 {frame.atMs} ms（预览帧，非逐帧 PTS 标定）</figcaption>
                </figure>
            )}
            <p className="text-xs text-muted-foreground">时间单位：毫秒。点击证据可定位原片；{report.audioAnalyzed ? "已分析音轨，仍需核对说话人和听不清的对白。" : "未分析音轨，当前对白仅供字幕校对。"}</p>
            {(["country", "language", "style"] as const).map((key) => (
                <label className="block" key={key}>
                    {{ country: "目标国家", language: "目标语言", style: "本地化风格" }[key]}
                    <Input disabled={busy} value={String(value[key] ?? "")} onChange={(e) => onChange({ ...value, [key]: e.target.value })} />
                </label>
            ))}
            {mode === "report" ? (
                <>
                    <details>
                        <summary>音乐、环境声与音效</summary>
                        {report.audioEvents?.map((event, index) => (
                            <div key={event.id} className="my-2">
                                <Button size="small" onClick={() => seek(event.startMs)}>
                                    {event.kind} · {event.startMs}–{event.endMs} ms
                                </Button>
                                <Input.TextArea
                                    aria-label={`声音 ${event.id}`}
                                    disabled={busy}
                                    value={event.description}
                                    onChange={(e) => update({ ...report, audioEvents: report.audioEvents?.map((v, i) => (i === index ? { ...v, description: e.target.value } : v)) })}
                                />
                            </div>
                        ))}
                    </details>
                    <details open>
                        <summary>人物、场景与道具</summary>
                        {report.entities.map((entity, index) => (
                            <div key={entity.id} className="my-2 space-y-1 rounded border border-border p-2">
                                <span>
                                    {entity.kind} · {entity.id}
                                </span>
                                <Input aria-label={`名称 ${entity.id}`} disabled={busy} value={entity.name} onChange={(e) => update({ ...report, entities: report.entities.map((item, i) => (i === index ? { ...item, name: e.target.value } : item)) })} />
                                <Input.TextArea
                                    aria-label={`描述 ${entity.id}`}
                                    disabled={busy}
                                    value={entity.description}
                                    onChange={(e) => update({ ...report, entities: report.entities.map((item, i) => (i === index ? { ...item, description: e.target.value } : item)) })}
                                />
                                {entity.evidence.map((e, i) => (
                                    <Button key={i} size="small" onClick={() => seek(e.atMs)}>
                                        {e.shotId} · {e.atMs} ms：{e.description}
                                    </Button>
                                ))}
                                {entity.kind === "character" && (
                                    <Select<string>
                                        aria-label={`合并人物 ${entity.id}`}
                                        className="w-full"
                                        placeholder="将此人物合并至…（会同步对白引用）"
                                        disabled={busy}
                                        value={undefined}
                                        options={options.filter((o) => o.value !== entity.id)}
                                        onChange={(id) => {
                                            try {
                                                update(mergeReportCharacters(report, entity.id, id));
                                                setError("");
                                            } catch (e) {
                                                setError(String(e));
                                            }
                                        }}
                                    />
                                )}
                            </div>
                        ))}
                    </details>
                    <details>
                        <summary>镜头与运镜</summary>
                        {report.shots.map((shot, index) => (
                            <div key={shot.id} className="my-2 space-y-1">
                                <Button size="small" onClick={() => seek(shot.startMs)}>
                                    {shot.id}
                                </Button>
                                {(["startMs", "endMs"] as const).map((key) => (
                                    <InputNumber
                                        key={key}
                                        aria-label={`${shot.id} ${key}`}
                                        disabled={busy}
                                        min={0}
                                        max={report.source.durationMs}
                                        value={shot[key]}
                                        onChange={(v) => v !== null && update({ ...report, shots: report.shots.map((s, i) => (i === index ? { ...s, [key]: v } : s)) })}
                                    />
                                ))}
                                {(["description", "camera"] as const).map((key) => (
                                    <Input.TextArea
                                        key={key}
                                        aria-label={`${shot.id} ${key}`}
                                        disabled={busy}
                                        value={shot[key]}
                                        onChange={(e) => update({ ...report, shots: report.shots.map((s, i) => (i === index ? { ...s, [key]: e.target.value } : s)) })}
                                    />
                                ))}
                            </div>
                        ))}
                    </details>
                    <details open>
                        <summary>对白校正</summary>
                        {report.dialogue.map((line, index) => (
                            <div key={line.id} className="my-2 space-y-1">
                                <Button size="small" onClick={() => seek(line.startMs)}>
                                    {line.id} · {line.evidenceKind}
                                </Button>
                                {(["startMs", "endMs"] as const).map((key) => (
                                    <InputNumber
                                        key={key}
                                        aria-label={`${line.id} ${key}`}
                                        disabled={busy}
                                        min={0}
                                        max={report.source.durationMs}
                                        value={line[key]}
                                        onChange={(v) => v !== null && update({ ...report, dialogue: report.dialogue.map((s, i) => (i === index ? { ...s, [key]: v } : s)) })}
                                    />
                                ))}
                                <Select
                                    aria-label={`说话人 ${line.id}`}
                                    disabled={busy}
                                    className="w-full"
                                    value={line.speakerId}
                                    options={[{ value: "", label: "说话人待确认" }, ...options]}
                                    onChange={(speakerId) => update({ ...report, dialogue: report.dialogue.map((s, i) => (i === index ? { ...s, speakerId } : s)) })}
                                />
                                <Input.TextArea aria-label={`对白 ${line.id}`} disabled={busy} value={line.text} onChange={(e) => update({ ...report, dialogue: report.dialogue.map((s, i) => (i === index ? { ...s, text: e.target.value } : s)) })} />
                                <Button size="small" disabled={busy} onClick={() => update({ ...report, dialogue: report.dialogue.filter((_, i) => i !== index) })}>
                                    删除此句
                                </Button>
                            </div>
                        ))}
                        <Button
                            disabled={busy || report.dialogue.length >= 300}
                            onClick={() =>
                                update({
                                    ...report,
                                    dialogue: [
                                        ...report.dialogue,
                                        { id: `line_${crypto.randomUUID()}`, startMs: report.coverage.startMs, endMs: report.coverage.endMs, text: "", speakerId: "", evidenceKind: report.audioAnalyzed ? "audio" : "subtitle", uncertainties: [] },
                                    ],
                                })
                            }
                        >
                            补充对白
                        </Button>
                    </details>
                </>
            ) : plan && Array.isArray(plan.mappings) && plan.mappings.every(Boolean) ? (
                <>
                    {plan.mappings.map((item, index) => (
                        <div key={`${item.kind}:${item.entityId}`} className="space-y-1 rounded border border-border p-2">
                            <p>
                                {item.kind} · {item.source}（{item.entityId}）
                            </p>
                            <label>
                                目标内容
                                <Input.TextArea disabled={busy} value={item.target} onChange={(e) => replaceMapping(index, { target: e.target.value })} />
                            </label>
                            <label>
                                推荐提示词
                                <Input.TextArea disabled={busy} value={item.prompt} onChange={(e) => replaceMapping(index, { prompt: e.target.value })} />
                            </label>
                            <label>
                                素材资源 ID
                                <Input
                                    disabled={busy}
                                    value={item.materialId}
                                    onChange={(e) => replaceMapping(index, { materialId: e.target.value })}
                                    onBlur={() => {
                                        if (item.materialId) void getResource(item.materialId).catch(() => setError("素材不可访问，请重新选择"));
                                    }}
                                />
                            </label>
                            <Select
                                className="w-full"
                                aria-label={`选择素材 ${item.entityId}`}
                                showSearch
                                optionFilterProp="label"
                                allowClear
                                placeholder="选择已上传素材"
                                disabled={busy}
                                value={item.materialId || undefined}
                                options={materials.filter((r) => (item.kind === "voice" ? r.kind === "audio" : ["image", "video"].includes(r.kind))).map((r) => ({ value: r.id, label: `${r.kind} · ${r.objectKey.split("/").pop() || r.id}` }))}
                                onChange={(id) => replaceMapping(index, { materialId: id || "" })}
                            />
                            <label>
                                上传替换素材
                                <input
                                    type="file"
                                    disabled={busy}
                                    accept={item.kind === "voice" ? "audio/*" : "image/*,video/*"}
                                    onChange={(e) => {
                                        const file = e.target.files?.[0];
                                        if (file) void material(index, file);
                                        e.target.value = "";
                                    }}
                                />
                            </label>
                            <p className="text-xs text-muted-foreground">{item.reason}</p>
                        </div>
                    ))}
                    <Checkbox disabled={busy} checked={value.confirmed === true} onChange={(e) => onChange({ ...value, confirmed: e.target.checked })}>
                        确认保存方案；此步骤不生成或替换成片
                    </Checkbox>
                </>
            ) : (
                <p role="alert">替换方案结构不完整，请切换 JSON 编辑。</p>
            )}
            {report.limitations.map((note, i) => (
                <p key={i} className="text-xs text-muted-foreground">
                    {note}
                </p>
            ))}
            {error && (
                <p role="alert" className="text-destructive">
                    {error}
                </p>
            )}
        </div>
    );
}
