import { http } from "@/services/api/request";
import type { PluginManifestV3, PluginOperation } from "@/lib/plugins/plugin-v3-types";

export type ApplicationRelease = {
    id: string;
    version: string;
    digest: string;
    revoked: boolean;
    manifest: PluginManifestV3;
    skills: Array<{ id: string; localSkillId: string; skillId: string; skillVersionId: string }>;
    operations: Array<PluginOperation & { available: boolean; reason: string }>;
    dependencyLock: string[];
};
export type ApplicationPlugin = {
    id: string;
    publisherId: string;
    installed: boolean;
    available: boolean;
    revision: number;
    releases: ApplicationRelease[];
    state: { enabled: boolean; effectiveEnabled: boolean; installedReleaseId: string; grantedPermissions: string[]; revision: number; reason: string };
};
export async function fetchApplicationPlugins(signal?: AbortSignal) {
    return http.get<{ applications: ApplicationPlugin[] }>("/plugins/applications", { signal });
}
export async function activateApplicationPlugin(id: string, input: { releaseId: string; enabled: boolean; grantedPermissions: string[]; revision: number }) {
    return http.put<{ updated: boolean }>(`/plugins/applications/${encodeURIComponent(id)}/activation`, input);
}
export async function manageApplicationPlugin(id: string, input: { action: "availability" | "uninstall" | "revoke"; available?: boolean; releaseId?: string; revision: number }) {
    return http.put<{ updated: boolean }>(`/admin/plugins/applications/${encodeURIComponent(id)}`, input);
}
