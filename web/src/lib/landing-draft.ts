const LANDING_DRAFT_KEY = "yingce:landing-draft";
const DRAFT_LIFETIME_MS = 30 * 60 * 1000;

// A short-lived, tab-local handoff survives the login redirect without putting the prompt in a URL.
export function saveLandingDraft(prompt: string, storage: Storage = sessionStorage, now = Date.now()) {
    storage.setItem(LANDING_DRAFT_KEY, JSON.stringify({ prompt: prompt.trim().slice(0, 2000), expiresAt: now + DRAFT_LIFETIME_MS }));
}

export function consumeLandingDraft(storage?: Storage, now = Date.now()): string {
    try {
        storage ??= sessionStorage;
        const raw = storage.getItem(LANDING_DRAFT_KEY);
        if (!raw) return "";
        storage.removeItem(LANDING_DRAFT_KEY);
        const value: unknown = JSON.parse(raw);
        if (!value || typeof value !== "object" || !("prompt" in value) || !("expiresAt" in value)) return "";
        if (typeof value.prompt !== "string" || typeof value.expiresAt !== "number" || value.expiresAt <= now) return "";
        return value.prompt.trim().slice(0, 2000);
    } catch {
        return "";
    }
}
