import { useEffect, useState } from "react";
import { getResource, resourceFileUrl } from "@/services/api/resources";
import { useUserStore } from "@/stores/use-user-store";

// Only host-owned resource IDs are accepted, never plugin-supplied URLs/HTML.
export function PluginMediaValue({ value, label }: { value: unknown; label: string }) {
    const userId = useUserStore((s) => s.user?.id);
    const candidate = typeof value === "string" ? value : value && typeof value === "object" && "resourceId" in value && typeof value.resourceId === "string" ? value.resourceId : "";
    const resourceId = /^[A-Za-z0-9_-]{1,160}$/.test(candidate) ? candidate : "";
    const scope = `${userId}:${resourceId}`;
    const [loaded, setLoaded] = useState<{ scope: string; kind?: string; error?: string }>();
    useEffect(() => {
        let active = true;
        if (resourceId && userId)
            void getResource(resourceId)
                .then((resource) => {
                    if (resource.status !== "ready" || !["image", "video", "audio"].includes(resource.kind)) throw new Error("媒体尚未就绪或格式不支持");
                    if (active) setLoaded({ scope, kind: resource.kind });
                })
                .catch((cause) => {
                    if (active) setLoaded({ scope, error: cause instanceof Error ? cause.message : "媒体读取失败" });
                });
        return () => {
            active = false;
        };
    }, [scope, resourceId, userId]);
    if (!resourceId) return <p role="alert">{label}：需要已导入的媒体资源</p>;
    const state = loaded?.scope === scope ? loaded : undefined;
    if (!state) return <p role="status">正在读取{label}…</p>;
    if (state.error)
        return (
            <p role="alert" className="text-destructive">
                {state.error}
            </p>
        );
    const src = resourceFileUrl(resourceId);
    const failed = () => setLoaded({ scope, error: "媒体加载失败，请检查资源权限与可用性" });
    if (state.kind === "image") return <img alt={label} src={src} onError={failed} className="max-h-72 w-full object-contain" loading="lazy" />;
    if (state.kind === "video") return <video aria-label={label} src={src} onError={failed} controls preload="metadata" className="max-h-72 w-full" />;
    return <audio aria-label={label} src={src} onError={failed} controls preload="metadata" className="w-full" />;
}
