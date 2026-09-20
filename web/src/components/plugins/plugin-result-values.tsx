// Shared by inline inspection, run receipts and canvas nodes; uses semantic text/border tokens.
import type { PluginResultView } from "@/services/api/plugin-operations";
import { pluginResultField, pluginResultFieldText } from "@/lib/plugins/plugin-result-view";
import { useState } from "react";
import { Button } from "antd";
import { PluginMediaValue } from "./plugin-media-value";

export function PluginResultValues({ value, view }: { value: unknown; view?: PluginResultView }) {
    const [page, setPage] = useState(0);
    if (!view) return <pre className="max-h-48 overflow-auto whitespace-pre-wrap break-all text-xs">{JSON.stringify(value, null, 2)}</pre>;
    if (view.component === "media-compare/v1")
        return (
            <div className="grid grid-cols-2 gap-3">
                {view.fields.map((field) => (
                    <figure key={field.path} className="min-w-0 space-y-2">
                        <figcaption>{field.label}</figcaption>
                        <PluginMediaValue value={pluginResultField(value, field.path)} label={field.label} />
                    </figure>
                ))}
            </div>
        );
    if (["table/v1", "entity-cards/v1", "mapping-editor/v1"].includes(view.component)) {
        const rows = view.collectionPath ? pluginResultField(value, view.collectionPath) : value;
        if (!Array.isArray(rows))
            return (
                <p role="alert" className="text-destructive">
                    组件需要数组数据，请检查视图的集合路径。
                </p>
            );
        const current = Math.min(page, Math.max(0, Math.ceil(rows.length / 20) - 1));
        const visible = rows.slice(current * 20, current * 20 + 20);
        return (
            <div className="space-y-2 overflow-auto" data-canvas-wheel-scroll>
                {rows.length === 0 ? (
                    <p>暂无条目</p>
                ) : view.component === "entity-cards/v1" ? (
                    <div className="grid gap-2">
                        {visible.map((row, i) => (
                            <article key={i} className="rounded-lg border border-border p-2">
                                <dl>
                                    {view.fields.map((field) => (
                                        <div key={field.path}>
                                            <dt className="text-muted-foreground">{field.label}</dt>
                                            <dd className="whitespace-pre-wrap break-all">{pluginResultFieldText(pluginResultField(row, field.path))}</dd>
                                        </div>
                                    ))}
                                </dl>
                            </article>
                        ))}
                    </div>
                ) : (
                    <table className="w-full text-left text-sm">
                        <thead>
                            <tr>
                                {view.fields.map((field) => (
                                    <th key={field.path} className="border-b border-border p-2">
                                        {field.label}
                                    </th>
                                ))}
                            </tr>
                        </thead>
                        <tbody>
                            {visible.map((row, i) => (
                                <tr key={i}>
                                    {view.fields.map((field) => (
                                        <td key={field.path} className="border-b border-border p-2 whitespace-pre-wrap break-all">
                                            {pluginResultFieldText(pluginResultField(row, field.path))}
                                        </td>
                                    ))}
                                </tr>
                            ))}
                        </tbody>
                    </table>
                )}
                {rows.length > 20 && (
                    <div className="flex items-center gap-2">
                        <Button disabled={current === 0} onClick={() => setPage(current - 1)}>
                            上一页
                        </Button>
                        <span>
                            第 {current + 1} 页，共 {rows.length} 项
                        </span>
                        <Button disabled={(current + 1) * 20 >= rows.length} onClick={() => setPage(current + 1)}>
                            下一页
                        </Button>
                    </div>
                )}
            </div>
        );
    }
    return (
        <dl className="space-y-2 text-sm" data-slot="plugin-result-values">
            {view.fields.map((field, index) => (
                <div key={`${field.path}:${index}`} className="grid grid-cols-2 gap-2 border-b border-border pb-1">
                    <dt className="text-muted-foreground">{field.label}</dt>
                    <dd className="break-all whitespace-pre-wrap">{pluginResultFieldText(pluginResultField(value, field.path))}</dd>
                </div>
            ))}
        </dl>
    );
}
