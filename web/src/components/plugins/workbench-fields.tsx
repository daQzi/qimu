import { Button, Input, Select } from "antd";
import { useEffect, useRef, useState } from "react";
import { BusinessObjectPicker } from "./business-object-picker";

// The input contract comes from the operation; no duplicate Skill form schema.
export function WorkbenchFields({
    schema,
    value,
    suggested,
    disabled,
    onChange,
    onValidityChange,
    objectInputs,
}: {
    objectInputs?: Record<string, "brand/v1">;
    schema: Record<string, unknown>;
    value: Record<string, unknown>;
    suggested: Record<string, unknown>;
    disabled: boolean;
    onChange: (value: Record<string, unknown>) => void;
    onValidityChange: (valid: boolean) => void;
}) {
    const [invalid, setInvalid] = useState<Record<string, boolean>>({});
    const [resets, setResets] = useState<Record<string, number>>({});
    const valid = !Object.values(invalid).some(Boolean);
    useEffect(() => onValidityChange(valid), [valid, onValidityChange]);
    const properties = (schema.properties || {}) as Record<string, Record<string, unknown>>;
    const required = Array.isArray(schema.required) ? schema.required : [];
    return (
        <div className="space-y-3">
            {Object.entries(properties).map(([name, field]) => {
                const current = Object.hasOwn(value, name) ? value[name] : Object.hasOwn(suggested, name) ? suggested[name] : undefined;
                const change = (next: unknown) => onChange({ ...value, [name]: next });
                const label = String(field.title || name);
                return (
                    <div key={name} className="block space-y-1">
                        <span>
                            {label}
                            {required.includes(name) ? " *" : ""}
                        </span>
                        {objectInputs?.[name] === "brand/v1" ? (
                            <BusinessObjectPicker value={current} disabled={disabled} onChange={change} />
                        ) : Array.isArray(field.enum) || field.type === "boolean" ? (
                            <Select
                                aria-label={label}
                                className="w-full"
                                disabled={disabled}
                                value={current === undefined ? undefined : JSON.stringify(current)}
                                options={(Array.isArray(field.enum) ? field.enum : [true, false]).map((v) => ({ value: JSON.stringify(v), label: String(v) }))}
                                onChange={(v) => change(JSON.parse(v))}
                                placeholder="请选择"
                            />
                        ) : ["string", "number", "integer"].includes(String(field.type)) ? (
                            <Input
                                aria-label={label}
                                disabled={disabled}
                                value={current === undefined ? "" : String(current)}
                                type={field.type === "string" ? "text" : "number"}
                                onChange={(e) => change(field.type === "string" ? e.target.value : e.target.value === "" ? null : Number(e.target.value))}
                            />
                        ) : (
                            <StructuredField key={`${name}:${resets[name] || 0}`} label={label} value={current} disabled={disabled} onChange={change} onInvalid={(bad) => setInvalid((previous) => ({ ...previous, [name]: bad }))} />
                        )}
                        {Object.hasOwn(value, name) ? (
                            <Button
                                size="small"
                                type="text"
                                disabled={disabled}
                                onClick={() => {
                                    const next = { ...value };
                                    delete next[name];
                                    setInvalid((previous) => ({ ...previous, [name]: false }));
                                    setResets((previous) => ({ ...previous, [name]: (previous[name] || 0) + 1 }));
                                    onChange(next);
                                }}
                            >
                                恢复建议值
                            </Button>
                        ) : null}
                        {typeof field.description === "string" ? <span className="block text-xs text-muted-foreground">{field.description}</span> : null}
                    </div>
                );
            })}
        </div>
    );
}

function StructuredField({ label, value, disabled, onChange, onInvalid }: { label: string; value: unknown; disabled: boolean; onChange: (value: unknown) => void; onInvalid: (invalid: boolean) => void }) {
    const [text, setText] = useState(JSON.stringify(value ?? null, null, 2));
    const [error, setError] = useState("");
    const serialized = JSON.stringify(value);
    const emitted = useRef(serialized);
    useEffect(() => {
        if (serialized !== emitted.current) {
            setText(JSON.stringify(value ?? null, null, 2));
            setError("");
            onInvalid(false);
            emitted.current = serialized;
        }
    }, [serialized]);
    return (
        <>
            <Input.TextArea
                aria-label={label}
                rows={4}
                disabled={disabled}
                value={text}
                onChange={(e) => {
                    setText(e.target.value);
                    try {
                        const parsed: unknown = JSON.parse(e.target.value);
                        emitted.current = JSON.stringify(parsed);
                        setError("");
                        onInvalid(false);
                        onChange(parsed);
                    } catch {
                        emitted.current = "null";
                        setError("请输入有效 JSON");
                        onInvalid(true);
                        onChange(null);
                    }
                }}
            />
            {error ? <span role="alert">{error}</span> : null}
        </>
    );
}
