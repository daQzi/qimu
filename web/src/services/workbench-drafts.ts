import { localForageStorageForScope } from "@/lib/localforage-storage";
import type { PluginInvocation, WorkbenchSelection } from "@/lib/plugins/plugin-v3-types";
import { validatePluginContract } from "@/lib/plugins/plugin-v3-contract";

export type WorkbenchDraft = { input: Record<string, unknown>; recipes: string[]; goal: string; runId?: string; pending?: { key: string; request: PluginInvocation }; agent?: WorkbenchSelection; agentGoal?: string };
const key = (id: string, release: string, canvas: string) => `workbench-draft-v1:${encodeURIComponent(id)}:${encodeURIComponent(release)}:${encodeURIComponent(canvas)}`;
export async function loadWorkbenchDraft(user: string, id: string, release: string, canvas: string): Promise<WorkbenchDraft | null> {
    const raw = await localForageStorageForScope(user).getItem(key(id, release, canvas));
    if (!raw) return null;
    try {
        if (raw.length > 128 * 1024) throw new Error("oversized draft");
        const value = JSON.parse(raw) as WorkbenchDraft;
        if (
            !value ||
            !value.input ||
            typeof value.input !== "object" ||
            Array.isArray(value.input) ||
            !Array.isArray(value.recipes) ||
            value.recipes.some((v) => typeof v !== "string") ||
            typeof value.goal !== "string" ||
            (value.pending && (typeof value.pending.key !== "string" || !value.pending.request || value.pending.request.workbench?.id !== id || value.pending.request.releaseId !== release))
        )
            throw new Error("工作台草稿损坏，请先核对运行历史，不能自动重试");
        if (value.recipes.length > 16 || Object.keys(value.input).length > 32 || value.goal.length > 16000 || (value.runId !== undefined && typeof value.runId !== "string") || (value.agentGoal !== undefined && typeof value.agentGoal !== "string"))
            throw new Error("invalid draft");
        if (value.agent) {
            validatePluginContract("workbenchSelection", JSON.stringify(value.agent));
            if (value.agent.id !== id || value.agent.releaseId !== release) throw new Error("agent context mismatch");
        }
        if (value.pending) {
            validatePluginContract("invocation", JSON.stringify(value.pending.request));
            if (value.pending.key.length < 8 || value.pending.key.length > 128 || (value.pending.request.context?.canvasId || "") !== canvas || value.pending.request.context?.hostSurface !== (canvas ? "canvas" : "agent-home"))
                throw new Error("pending context mismatch");
        }
        return value;
    } catch {
        throw new Error("工作台草稿损坏，请先核对运行历史，不能自动重试");
    }
}
export const saveWorkbenchDraft = (user: string, id: string, release: string, canvas: string, draft: WorkbenchDraft) => localForageStorageForScope(user).setItem(key(id, release, canvas), JSON.stringify(draft));
