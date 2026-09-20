import { afterAll, expect, test } from "bun:test";
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

// Isolate only IndexedDB; execute the production recovery code with separate user stores.
const dir = mkdtempSync(join(tmpdir(), "thread-pending-"));
writeFileSync(join(dir, "storage.ts"), `const stores = new Map(); export function localForageStorageForScope(scope) { if (!stores.has(scope)) stores.set(scope, new Map()); const store = stores.get(scope); return { getItem: async k => store.get(k), setItem: async (k,v) => store.set(k,v), removeItem: async k => store.delete(k) }; }`);
writeFileSync(join(dir, "pending.ts"), readFileSync(new URL("../src/services/agent-thread-pending.ts", import.meta.url), "utf8").replace('"@/lib/localforage-storage"', '"./storage"'));
const pending = await import(join(dir, "pending.ts")) as typeof import("../src/services/agent-thread-pending");
afterAll(() => rmSync(dir, { recursive: true, force: true }));

test("pending turn keeps its original canvas and revision across hosts, isolated by account", async () => {
    const record = { key: "pending-original-key", fingerprint: "frozen-request", threadRevision: 3, request: { canvasId: "", hostSurface: "agent-home" as const, prompt: "原消息", idempotencyKey: "pending-original-key" } };
    await pending.saveThreadPending("alice", "thread", record);
    expect(await pending.loadThreadPending("alice", "thread")).toEqual(record);
    expect(await pending.loadThreadPending("bob", "thread")).toBeNull();
    expect(await pending.loadThreadPending("alice", "another-thread")).toBeNull();
    await pending.clearThreadPending("bob", "thread");
    expect(await pending.loadThreadPending("alice", "thread")).toEqual(record);
    await pending.clearThreadPending("alice", "thread");
    expect(await pending.loadThreadPending("alice", "thread")).toBeNull();
});

test("a corrupt or incomplete revision cannot become a fresh chargeable request", async () => {
    await pending.saveThreadPending("alice", "corrupt", { key: "pending-key", fingerprint: "frozen", request: { prompt: "message", idempotencyKey: "pending-key" } });
    expect(pending.loadThreadPending("alice", "corrupt")).rejects.toThrow("待确认消息记录损坏");
});
