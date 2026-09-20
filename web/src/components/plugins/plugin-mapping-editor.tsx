import { Button, Input, Select } from "antd";
import { useState } from "react";
import type { PluginResultView } from "@/services/api/plugin-operations";
import { mappingKey, mappingRows, mappingSchema } from "@/lib/plugins/plugin-mapping";

export function PluginMappingEditor({ value, schema, view, disabled, onChange }: { value: unknown; schema: Record<string, unknown>; view: PluginResultView; disabled: boolean; onChange: (value: unknown) => void }) {
    const [page, setPage] = useState(0);
    const shape = mappingSchema(schema, view);
    const rows = mappingRows(value, view);
    if (!shape || !rows || !value || typeof value !== "object" || Array.isArray(value)) return <p role="alert">该数据或 Schema 暂不支持表格编辑，请切换 JSON 编辑。</p>;
    const save = (next: Record<string, unknown>[]) => onChange({ ...value, [shape.key]: next });
    const current = Math.min(page, Math.max(0, Math.ceil(rows.length / 10) - 1));
    return (
        <div className="space-y-2 overflow-auto" data-canvas-wheel-scroll>
            <table className="w-full text-left">
                <thead>
                    <tr>
                        {view.fields.map((field) => (
                            <th className="p-1" key={field.path}>
                                {field.label}
                            </th>
                        ))}
                        <th>操作</th>
                    </tr>
                </thead>
                <tbody>
                    {rows.slice(current * 10, current * 10 + 10).map((row, offset) => {
                        const index = current * 10 + offset;
                        return (
                            <tr key={index}>
                                {view.fields.map((field) => {
                                    const key = mappingKey(field.path),
                                        prop = shape.fields[key];
                                    const change = (next: unknown) => {
                                        const updated = { ...row };
                                        if (next === undefined) delete updated[key];
                                        else updated[key] = next;
                                        save(rows.map((r, i) => (i === index ? updated : r)));
                                    };
                                    return (
                                        <td key={field.path} className="min-w-28 p-1">
                                            {Array.isArray(prop.enum) || prop.type === "boolean" ? (
                                                <Select
                                                    aria-label={`${index + 1} ${field.label}`}
                                                    className="w-full"
                                                    disabled={disabled}
                                                    allowClear
                                                    value={row[key] === undefined ? undefined : JSON.stringify(row[key])}
                                                    options={(Array.isArray(prop.enum) ? prop.enum : [true, false]).map((v) => ({ label: String(v), value: JSON.stringify(v) }))}
                                                    onChange={(v) => change(v === undefined ? undefined : JSON.parse(v))}
                                                />
                                            ) : (
                                                <Input
                                                    aria-label={`${index + 1} ${field.label}`}
                                                    disabled={disabled}
                                                    type={prop.type === "string" ? "text" : "number"}
                                                    value={row[key] === undefined ? "" : String(row[key])}
                                                    onChange={(e) => change(prop.type === "string" ? e.target.value : e.target.value === "" ? undefined : Number(e.target.value))}
                                                />
                                            )}
                                        </td>
                                    );
                                })}
                                <td>
                                    <Button disabled={disabled} onClick={() => save(rows.filter((_, i) => i !== index))}>
                                        删除行
                                    </Button>
                                </td>
                            </tr>
                        );
                    })}
                </tbody>
            </table>
            <div className="flex flex-wrap gap-2">
                <Button
                    disabled={disabled || rows.length >= shape.maxItems}
                    onClick={() => {
                        save([...rows, {}]);
                        setPage(Math.floor(rows.length / 10));
                    }}
                >
                    添加行
                </Button>
                <Button disabled={current === 0} onClick={() => setPage(current - 1)}>
                    上一页
                </Button>
                <span>
                    第 {current + 1} 页 · {rows.length} 项
                </span>
                <Button disabled={(current + 1) * 10 >= rows.length} onClick={() => setPage(current + 1)}>
                    下一页
                </Button>
            </div>
        </div>
    );
}
