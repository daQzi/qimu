import type { CanvasNodeData } from "@/types/canvas";
import { PluginRunCard } from "@/components/plugins/plugin-run-card";

export function PluginInputNodeContent({ node }: { node: CanvasNodeData }) {
    const binding = node.metadata?.pluginInput;
    if (!binding?.runId || !binding.inputRequestId)
        return (
            <p role="alert" className="p-3 text-destructive">
                缺少输入请求绑定，请从插件运行重新创建节点。
            </p>
        );
    return (
        <div className="h-full overflow-auto" data-canvas-no-zoom data-canvas-wheel-scroll onPointerDown={(event) => event.stopPropagation()} onKeyDown={(event) => event.stopPropagation()}>
            <PluginRunCard key={`${binding.runId}:${binding.inputRequestId}`} runId={binding.runId} inputRequestId={binding.inputRequestId} expectedReleaseId={binding.releaseId} canvasId={binding.canvasId} />
        </div>
    );
}
