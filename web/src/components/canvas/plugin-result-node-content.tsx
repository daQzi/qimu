import type { CanvasNodeData } from "@/types/canvas";
import { PluginRunCard } from "@/components/plugins/plugin-run-card";

export function PluginResultNodeContent({ node }: { node: CanvasNodeData }) {
    const binding = node.metadata?.pluginResult;
    if (!binding?.runId || !binding.digest) return <p role="alert" className="p-3 text-destructive">缺少成功结果绑定，请从插件运行重新保存。</p>;
    return <div className="h-full overflow-auto" data-canvas-no-zoom data-canvas-wheel-scroll onPointerDown={(event) => event.stopPropagation()}>
        <PluginRunCard key={`${binding.runId}:${binding.digest}`} runId={binding.runId} expectedDigest={binding.digest} viewId={binding.viewId} />
    </div>;
}
