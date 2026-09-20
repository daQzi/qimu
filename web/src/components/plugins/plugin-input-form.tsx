import { Button, Input, Select } from "antd";
import { useRef, useState } from "react";
import { updatePluginInput, type PluginInputRequest, type PluginRunView } from "@/services/api/plugin-operations";
import { useUserStore } from "@/stores/use-user-store";
import { PluginMappingEditor } from "./plugin-mapping-editor";

// Primitive fields have ordinary controls. Structured/ref schemas use explicit
// JSON, with the same authoritative server validation; no guessed defaults.
export function PluginInputForm({ run, input, onUpdated }: { run: PluginRunView; input: PluginInputRequest; onUpdated: (run: PluginRunView) => void }) {
    const userID = useUserStore((s) => s.user?.id);
    const [text, setText] = useState(JSON.stringify(input.draft ?? {}, null, 2));
    const [error, setError] = useState("");
    const [busy, setBusy] = useState(false);
    const [jsonMode, setJSONMode] = useState(false);
    const attempt = useRef<{ signature: string; key: string } | undefined>(undefined);
    const props = input.schema.properties as Record<string, Record<string, unknown>> | undefined;
    const simple = input.schema.type === "object" && props && !input.schema.$ref && Object.values(props).every((p) => ["string", "boolean", "number", "integer"].includes(String(p.type)) && !p.$ref);
    const parsedValue: unknown = (() => {
        try {
            return JSON.parse(text);
        } catch {
            return undefined;
        }
    })();
    const objectValue = !!parsedValue && typeof parsedValue === "object" && !Array.isArray(parsedValue);
    const parsed = (objectValue ? parsedValue : {}) as Record<string, unknown>;
    const required = Array.isArray(input.schema.required) ? input.schema.required : [];
    const field = (name: string, value: unknown) => {
        const next = { ...parsed };
        if (value === undefined) delete next[name];
        else Object.defineProperty(next, name, { value, enumerable: true, configurable: true, writable: true });
        setText(JSON.stringify(next, null, 2));
    };
    const save = async (mode: "draft" | "submit") => {
        if (busy) return;
        setBusy(true);
        setError("");
        try {
            const value: unknown = JSON.parse(text);
            const signature = JSON.stringify({ value, revision: input.revision, mode });
            if (attempt.current?.signature !== signature) attempt.current = { signature, key: crypto.randomUUID() };
            const result = await updatePluginInput(run.id, input, value, mode, attempt.current.key);
            if (useUserStore.getState().user?.id === userID) onUpdated(result);
        } catch (e) {
            if (useUserStore.getState().user?.id === userID) setError(e instanceof Error ? e.message : "保存失败");
        } finally {
            if (useUserStore.getState().user?.id === userID) setBusy(false);
        }
    };
    return (
        <section aria-label="流程输入" className="space-y-2">
            <p>请补充：{input.stepKey}。提交内容后，收费或外部操作仍需单独确认。</p>
            {input.view?.component === "mapping-editor/v1" && (
                <Button size="small" aria-pressed={jsonMode} onClick={() => setJSONMode(!jsonMode)}>
                    {jsonMode ? "表格编辑" : "JSON 编辑"}
                </Button>
            )}
            {input.view?.component === "mapping-editor/v1" && !jsonMode ? (
                <PluginMappingEditor value={parsedValue} schema={input.schema} view={input.view} disabled={busy} onChange={(value) => setText(JSON.stringify(value, null, 2))} />
            ) : simple && !jsonMode && objectValue ? (
                Object.entries(props).map(([name, schema]) => (
                    <label key={name} className="block space-y-1">
                        <span>
                            {input.view?.fields.find((field) => field.path === `/${name.replace(/~/g, "~0").replace(/\//g, "~1")}`)?.label || String(schema.title || name)}
                            {required.includes(name) ? " *" : ""}
                        </span>
                        {Array.isArray(schema.enum) || schema.type === "boolean" ? (
                            <Select
                                aria-label={name}
                                className="w-full"
                                allowClear
                                disabled={busy}
                                value={parsed[name] === undefined ? undefined : JSON.stringify(parsed[name])}
                                placeholder="请选择"
                                options={(Array.isArray(schema.enum) ? schema.enum : [true, false]).map((v) => ({ value: JSON.stringify(v), label: String(v) }))}
                                onChange={(v) => field(name, v === undefined ? undefined : JSON.parse(v))}
                            />
                        ) : (
                            <Input
                                aria-label={name}
                                disabled={busy}
                                type={schema.type === "string" ? "text" : "number"}
                                value={parsed[name] === undefined ? "" : String(parsed[name])}
                                onChange={(e) => field(name, schema.type === "string" ? e.target.value : e.target.value === "" ? undefined : Number(e.target.value))}
                            />
                        )}
                    </label>
                ))
            ) : (
                <Input.TextArea aria-label="流程输入 JSON" rows={5} value={text} disabled={busy} onChange={(e) => setText(e.target.value)} />
            )}
            <details>
                <summary>输入格式</summary>
                <pre className="max-h-48 overflow-auto whitespace-pre-wrap">{JSON.stringify(input.schema, null, 2)}</pre>
                {!simple && <pre className="max-h-48 overflow-auto whitespace-pre-wrap">{JSON.stringify(run.pipeline?.schemas, null, 2)}</pre>}
            </details>
            <div className="flex gap-2">
                <Button disabled={busy} onClick={() => void save("draft")}>
                    保存草稿
                </Button>
                <Button type="primary" loading={busy} onClick={() => void save("submit")}>
                    提交并继续
                </Button>
            </div>
            {error && (
                <p role="alert" className="text-destructive">
                    {error}
                </p>
            )}
        </section>
    );
}
