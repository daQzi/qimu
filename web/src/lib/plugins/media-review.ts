export type VideoReport = {
    schemaVersion: number;
    source: { resourceId: string; digest: string; durationMs: number };
    coverage: { startMs: number; endMs: number };
    audioAnalyzed: boolean;
    audioEvents?: { id: string; startMs: number; endMs: number; kind: string; description: string; uncertainties: string[] }[];
    shots: { id: string; startMs: number; endMs: number; description: string; camera: string; uncertainties: string[] }[];
    entities: { id: string; kind: string; name: string; description: string; evidence: { shotId: string; atMs: number; description: string }[]; uncertainties: string[] }[];
    dialogue: { id: string; startMs: number; endMs: number; text: string; speakerId: string; evidenceKind: string; uncertainties: string[] }[];
    limitations: string[];
};
export type VideoPlan = { mappings: { entityId: string; kind: string; source: string; target: string; prompt: string; materialId: string; executable: boolean; reason: string }[]; limitations: string[] };

// Preserve the surviving identity and every source observation, including lines
// spoken by the duplicate. Reject invalid merges instead of dropping evidence.
export function mergeReportCharacters(report: VideoReport, from: string, into: string): VideoReport {
    const source = report.entities.find((e) => e.id === from);
    const target = report.entities.find((e) => e.id === into);
    if (from === into || source?.kind !== "character" || target?.kind !== "character") throw new Error("请选择两个不同的人物");
    const evidence = Array.from(new Map([...target.evidence, ...source.evidence].map((e) => [JSON.stringify(e), e])).values());
    const uncertainties = Array.from(new Set([...target.uncertainties, ...source.uncertainties]));
    if (evidence.length > 20 || uncertainties.length > 20) throw new Error("合并后证据超过表单上限，请先在 JSON 模式整理证据");
    return { ...report, entities: report.entities.filter((e) => e.id !== from).map((e) => (e.id === into ? { ...e, evidence, uncertainties } : e)), dialogue: report.dialogue.map((line) => (line.speakerId === from ? { ...line, speakerId: into } : line)) };
}
