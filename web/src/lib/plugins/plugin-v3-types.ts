/** P00 wire contracts only; not part of RegisteredPlugin or the live installer. */
export type PluginInvocationResult = { kind: "inline"; result: unknown; digest?: string } | { kind: "run"; runId: string; status: PluginRunStatus };
export type PluginStepRecord = {
    id: string;
    runId: string;
    stepKey: string;
    itemKey: string;
    attempt: number;
    status: PluginStepStatus;
    inputDigest: string;
    taskId?: string;
    submissionState: "not_started" | "prepared" | "submitted" | "unknown" | "upstream_completed" | "import_pending" | "imported";
    outputRefs: PluginResultRef[];
};
export type PluginV3Permission =
    "canvas.read" | "canvas.write" | "media.read" | "asset.read" | "asset.search" | "asset.import" | "asset.upload" | "generation.run" | "timeline.read" | "timeline.command" | "export.run" | "connection.use" | "resource.create";
export type PluginV3Effect = "read" | "draft_write" | "generation" | "external_write" | "delete" | "publish";
export type PluginRunStatus = "queued" | "running" | "waiting_input" | "waiting_approval" | "paused" | "succeeded" | "failed" | "cancelling" | "cancelled";
export type PluginStepStatus = "pending" | "ready" | "running" | "waiting_task" | "waiting_input" | "waiting_approval" | "succeeded" | "failed" | "skipped" | "cancelled";
export type PluginContributionRef = { id: string; ref: string };
export type PluginV3Skill = { id: string; name: string; description: string; entry: string; activation: "auto" | "explicit"; operations: string[] };
export type PluginManifestV3 = {
    apiVersion: "yingce.plugin/v3";
    id: string;
    name: string;
    version: string;
    description: string;
    publisher: { id: string; displayName: string };
    requires: { hostApi: "^3.0.0" };
    permissions: PluginV3Permission[];
    dependencies: Array<{ id: string; version: string; optional: boolean }>;
    contributes: { skills?: PluginV3Skill[]; operations?: PluginContributionRef[]; views?: PluginContributionRef[]; canvasBlueprints?: PluginContributionRef[] };
};
export type PluginExecution = { kind: "host"; adapter: string; mode: "inline" | "task" } | { kind: "http"; connector: string; action: string; mode: "task" } | { kind: "pipeline"; pipeline: string; mode: "task" };
export type PluginOperation = {
    id: string;
    description: string;
    inputSchemaRef: string;
    outputSchemaRef: string;
    requiredPermissions: PluginV3Permission[];
    effects: PluginV3Effect[];
    context: { requiresCanvas: boolean; requiresProject: boolean };
    execution: PluginExecution;
    resultView?: string;
};
export type PluginInvocationContext = { hostSurface?: "agent-home" | "canvas" | "editor"; canvasId?: string; projectId?: string; threadId?: string; workbenchId?: string };
export type PluginInvocation = { operation: string; releaseId: string; input: Record<string, unknown>; context?: PluginInvocationContext };
export type PluginResultRef = { runId: string; stepKey: string; itemKey: string; attempt: number; outputKey: string; schemaId: string; schemaVersion: string; digest: string; resourceId?: string };
export type PluginSkillBinding = { releaseId: string; localSkillId: string; skillVersionId: string; entryDigest: string };
export type PluginRunRecord = { id: string; userId: string; entryOperation: string; releaseId: string; idempotencyKey: string; requestDigest: string; status: PluginRunStatus; revision: number; context: PluginInvocationContext; parentRunId?: string };
export type PluginBinding = { literal: unknown; from?: never } | { from: string; literal?: never };
export type PluginPipelineStep = { key: string; dependsOn: string[] } & (
    | { type: "operation"; operation: string; inputs: Record<string, PluginBinding>; when?: { exists: string } | { equals: [PluginBinding, PluginBinding] }; foreach?: { from: string; itemKey: string; maxConcurrency: number } }
    | { type: "wait_input"; formSchemaRef: string; view?: string }
);
export type PluginPipeline = { id: string; inputSchemaRef: string; outputSchemaRef: string; steps: PluginPipelineStep[]; outputs: Record<string, PluginBinding> };
