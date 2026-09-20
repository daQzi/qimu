import { http } from "./request";
import type { PluginInvocationContext, WorkbenchSelection } from "@/lib/plugins/plugin-v3-types";

export type WorkbenchDefinition = {
    objectInputs?: Record<string, "brand/v1">;
    id: string;
    name: string;
    description: string;
    hostSurfaces: string[];
    contextSchemaRef: string;
    operation?: string;
    skill?: string;
    recipes: string[];
    defaults: Record<string, unknown>;
    output: "result" | "canvas_optional";
    prompt?: string;
};
export type WorkbenchRecipe = { id: string; name: string; defaults: Record<string, unknown>; requirements: Record<string, unknown>; prompt?: string };
export type WorkbenchView = { id: string; releaseId: string; version: string; definition: WorkbenchDefinition; recipes: Record<string, WorkbenchRecipe>; schema: Record<string, unknown>; schemas: Record<string, unknown>; skillId?: string };
export type WorkbenchPreview = {
    objects?: Record<string, import("./business-objects").ObjectView>;
    input: Record<string, unknown>;
    prompts: string[];
    conflicts: Array<{ field: string; message: string }>;
    selection: WorkbenchSelection;
    valid: boolean;
    validationMessage?: string;
};
export const listWorkbenches = () => http.get<WorkbenchView[]>("/plugin-workbenches");
export const getWorkbench = (id: string, releaseId: string, context: PluginInvocationContext) => http.get<WorkbenchView>(`/plugin-workbenches/${encodeURIComponent(id)}`, { params: { releaseId, ...context } });
export const previewWorkbench = (id: string, releaseId: string, recipeIds: string[], input: Record<string, unknown>, context: PluginInvocationContext) =>
    http.post<WorkbenchPreview>("/plugin-workbench-previews", { id, releaseId, recipeIds, input, context });
