import type { CanvasNodeData } from "@/types/canvas";
import { PluginRunCard } from "@/components/plugins/plugin-run-card";
import { Button } from "antd";
import { useState } from "react";
import { requestPluginEditorAction } from "@/lib/plugins/plugin-editor-actions";

export function PluginResultNodeContent({ node }: { node: CanvasNodeData }) {
    const binding = node.metadata?.pluginResult;
    const [error, setError] = useState("");
    if (!binding?.runId || !binding.digest)
        return (
            <p role="alert" className="p-3 text-destructive">
                缺少成功结果绑定，请从插件运行重新保存。
            </p>
        );
    return (
        <div className="h-full overflow-auto" data-canvas-no-zoom data-canvas-wheel-scroll onPointerDown={(event) => event.stopPropagation()} onKeyDown={(event) => event.stopPropagation()}>
            {binding.canvasId && binding.actions?.includes("result.continue") && (
                <Button
                    className="m-2"
                    onClick={() => {
                        try {
                            requestPluginEditorAction({ command: "result.continue", canvasId: binding.canvasId!, runId: binding.runId, nodeId: node.id });
                            setError("");
                        } catch (cause) {
                            setError(cause instanceof Error ? cause.message : "无法定位节点");
                        }
                    }}
                >
                    选中结果，继续与 Agent 对话
                </Button>
            )}
            {error && (
                <p role="alert" className="text-destructive">
                    {error}
                </p>
            )}
            <PluginRunCard key={`${binding.runId}:${binding.digest}`} runId={binding.runId} expectedDigest={binding.digest} expectedReleaseId={binding.releaseId} viewId={binding.viewId} />
        </div>
    );
}
