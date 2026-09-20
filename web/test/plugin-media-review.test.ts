import { expect, test } from "bun:test";
import { mergeReportCharacters, type VideoReport } from "../src/lib/plugins/media-review";

test("merge keeps evidence and rewrites dialogue without mutating original", () => {
    const entity = (id: string) => ({ id, kind: "character", name: id, description: id, evidence: [{ shotId: "shot", atMs: 0, description: id }], uncertainties: [] });
    const report: VideoReport = {
        schemaVersion: 1,
        source: { resourceId: "video", digest: "a".repeat(64), durationMs: 1000 },
        coverage: { startMs: 0, endMs: 1000 },
        audioAnalyzed: true,
        shots: [],
        entities: [entity("a"), entity("b")],
        dialogue: [{ id: "line", startMs: 0, endMs: 1000, text: "hello", speakerId: "a", evidenceKind: "audio", uncertainties: [] }],
        limitations: [],
    };
    const next = mergeReportCharacters(report, "a", "b");
    expect(next.entities.length).toBe(1);
    expect(next.entities[0].evidence.length).toBe(2);
    expect(next.dialogue[0].speakerId).toBe("b");
    expect(report.dialogue[0].speakerId).toBe("a");
    expect(() => mergeReportCharacters(report, "a", "a")).toThrow();
    expect(() => mergeReportCharacters(report, "missing", "b")).toThrow();
});
