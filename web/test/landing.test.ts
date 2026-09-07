import { describe, expect, test } from "bun:test";
import { consumeLandingDraft, saveLandingDraft } from "../src/lib/landing-draft";
import { resolveLandingMedia } from "../src/pages/landing/landing-media";

function memoryStorage(): Storage {
    const data = new Map<string, string>();
    return {
        get length() { return data.size; },
        clear: () => data.clear(),
        key: (index) => [...data.keys()][index] ?? null,
        getItem: (key) => data.get(key) ?? null,
        setItem: (key, value) => { data.set(key, value); },
        removeItem: (key) => { data.delete(key); },
    };
}

describe("landing draft handoff", () => {
    test("survives a login redirect and can only be consumed once", () => {
        const storage = memoryStorage();
        saveLandingDraft("  黎明前的城市  ", storage, 100);
        expect(consumeLandingDraft(storage, 200)).toBe("黎明前的城市");
        expect(consumeLandingDraft(storage, 201)).toBe("");
    });
    test("discards expired, malformed and unavailable drafts", () => {
        const storage = memoryStorage();
        saveLandingDraft("旧草稿", storage, 0);
        expect(consumeLandingDraft(storage, 30 * 60 * 1000)).toBe("");
        storage.setItem("yingce:landing-draft", "{bad");
        expect(consumeLandingDraft(storage)).toBe("");
        expect(consumeLandingDraft({ ...storage, getItem() { throw new Error("denied"); } })).toBe("");
    });
    test("limits transferred text and reports save failures", () => {
        const storage = memoryStorage();
        saveLandingDraft("x".repeat(2100), storage);
        expect(consumeLandingDraft(storage)).toHaveLength(2000);
        expect(() => saveLandingDraft("草稿", { ...storage, setItem() { throw new Error("denied"); } })).toThrow();
    });
});

describe("landing background media", () => {
    test("uses the configured video and poster without replacing them", () => {
        expect(resolveLandingMedia({ authVideoUrl: "/api/public/appearance/assets/video?v=2", authVideoPosterUrl: "/poster.jpg" }))
            .toEqual({ kind: "video", src: "/api/public/appearance/assets/video?v=2", poster: "/poster.jpg" });
    });
    test("supports a still image and a local fallback", () => {
        expect(resolveLandingMedia({ authVideoUrl: "", authVideoPosterUrl: "/poster.jpg" })).toEqual({ kind: "image", src: "/poster.jpg" });
        expect(resolveLandingMedia({ authVideoUrl: "", authVideoPosterUrl: "" })).toEqual({ kind: "image", src: "/short-drama-styles/future-tech.jpg" });
    });
});
