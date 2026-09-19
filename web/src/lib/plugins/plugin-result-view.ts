// A view is data, never code. Resolve only own JSON properties and RFC 6901 escapes.
export function pluginResultField(value: unknown, pointer: string): unknown {
    if (!pointer.startsWith("/")) return undefined;
    let current = value;
    for (const segment of pointer.slice(1).split("/")) {
        if (/~[^01]|~$/.test(segment)) return undefined;
        const key = segment.replace(/~1/g, "/").replace(/~0/g, "~");
        if (current === null || typeof current !== "object" || !Object.hasOwn(current, key)) return undefined;
        current = (current as Record<string, unknown>)[key];
    }
    return current;
}

export function pluginResultFieldText(value: unknown): string {
    if (value === undefined) return "未记录";
    if (value === null) return "空值";
    if (typeof value === "string") return value;
    return JSON.stringify(value);
}
