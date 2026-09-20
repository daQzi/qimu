import { localForageStorageForScope } from "@/lib/localforage-storage";
import type { CloudAgentPendingSubmission } from "./cloud-agent-conversations";

const key = (threadId: string) => `agent-thread-pending-v1:${encodeURIComponent(threadId)}`;
export async function loadThreadPending(userId: string, threadId: string): Promise<CloudAgentPendingSubmission | null> {
    const raw = await localForageStorageForScope(userId).getItem(key(threadId));
    if (raw == null) return null;
    try {
        const value = JSON.parse(raw) as CloudAgentPendingSubmission;
        if (!value || typeof value.key !== "string" || value.key.length < 8 || typeof value.fingerprint !== "string" || !value.request || value.request.idempotencyKey !== value.key || typeof value.request.prompt !== "string" || !Number.isSafeInteger(value.threadRevision) || value.threadRevision! < 1) throw new Error("invalid pending request");
        return value;
    } catch { throw new Error("待确认消息记录损坏，请先核对会话运行，不能直接重复提交"); }
}
export const saveThreadPending = (userId: string, threadId: string, pending: CloudAgentPendingSubmission) => localForageStorageForScope(userId).setItem(key(threadId), JSON.stringify(pending));
export const clearThreadPending = (userId: string, threadId: string) => localForageStorageForScope(userId).removeItem(key(threadId));
