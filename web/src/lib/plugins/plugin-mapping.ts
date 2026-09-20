import { pluginResultField } from "./plugin-result-view";
import type { PluginResultView } from "@/services/api/plugin-operations";

export function mappingKey(path: string) {
    if (!/^\/[^/]+$/.test(path) || /~(?![01])/.test(path)) throw new Error("映射字段仅支持单层 JSON Pointer");
    const key = path.slice(1).replace(/~1/g, "/").replace(/~0/g, "~");
    if (["__proto__", "constructor", "prototype"].includes(key)) throw new Error("不支持的字段名");
    return key;
}
export function mappingSchema(schema: Record<string, unknown>, view: PluginResultView) {
    try {
        const key = mappingKey(view.collectionPath || "");
        const props = schema.properties as Record<string, Record<string, unknown>>;
        const array = props?.[key];
        const items = array?.items as Record<string, unknown>;
        const fields = items?.properties as Record<string, Record<string, unknown>>;
        if (schema.type !== "object" || schema.$ref || array?.type !== "array" || items?.type !== "object" || !fields) return;
        if (!view.fields.every((field) => ["string", "number", "integer", "boolean"].includes(String(fields[mappingKey(field.path)]?.type)) && !fields[mappingKey(field.path)].$ref)) return;
        return { key, fields, maxItems: typeof array.maxItems === "number" ? Math.min(array.maxItems, 256) : 256 };
    } catch {
        return;
    }
}
export function mappingRows(value: unknown, view: PluginResultView): Record<string, unknown>[] | undefined {
    const rows = pluginResultField(value, view.collectionPath || "");
    if (rows === undefined) return [];
    if (!Array.isArray(rows) || rows.some((row) => !row || typeof row !== "object" || Array.isArray(row))) return;
    return rows;
}
