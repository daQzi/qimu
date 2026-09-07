const workspaceRouteLoaders = {
    assets: () => import("@/pages/assets"),
    canvas: () => import("@/pages/canvas"),
    create: () => import("@/pages/create"),
    projects: () => import("@/pages/projects"),
    projectDetail: () => import("@/pages/projects/detail"),
    wallet: () => import("@/pages/wallet"),
};

export const loadAssetsPage = workspaceRouteLoaders.assets;
export const loadCanvasPage = workspaceRouteLoaders.canvas;
export const loadCanvasProjectPage = () => import("@/pages/canvas/project");
export const loadCreatePage = workspaceRouteLoaders.create;
export const loadProjectDetailPage = workspaceRouteLoaders.projectDetail;
export const loadProjectsPage = workspaceRouteLoaders.projects;
export const loadWalletPage = workspaceRouteLoaders.wallet;

export function preloadWorkspaceRoute(pathnameOrSlug: string) {
    // 创作首页和 /create 共用同一模块。
    const segments = pathnameOrSlug.replace(/^\//, "").split("/").filter(Boolean);
    const slug = segments[0] === "home" ? "create" : segments[0];
    if (slug === "projects" && segments.length > 1) {
        void workspaceRouteLoaders.projectDetail();
        return;
    }
    const load = workspaceRouteLoaders[slug as keyof typeof workspaceRouteLoaders];
    if (load) void load();
}
