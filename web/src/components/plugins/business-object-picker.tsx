import { useEffect, useState } from "react";
import { Button, Input, Select } from "antd";
import { Link } from "react-router";
import { listBusinessObjects, readBusinessObject, type BusinessObject, type ObjectReference, type ObjectView } from "@/services/api/business-objects";
import { useUserStore } from "@/stores/use-user-store";

export function BusinessObjectPicker({ value, disabled, onChange }: { value: unknown; disabled: boolean; onChange: (ref: ObjectReference) => void }) {
    const user = useUserStore((s) => s.user?.id);
    return <OwnedPicker key={user || "anonymous"} user={user} value={value} disabled={disabled} onChange={onChange} />;
}
function OwnedPicker({ user, value, disabled, onChange }: { user?: string; value: unknown; disabled: boolean; onChange: (ref: ObjectReference) => void }) {
    const [rows, setRows] = useState<BusinessObject[]>([]);
    const [query, setQuery] = useState("");
    const [search, setSearch] = useState("");
    const [refresh, setRefresh] = useState(0);
    const [offset, setOffset] = useState(0);
    const [error, setError] = useState("");
    const [view, setView] = useState<ObjectView>();
    const [loading, setLoading] = useState(false);
    const ref = value && typeof value === "object" && "objectId" in value ? (value as ObjectReference) : undefined;
    useEffect(() => {
        const controller = new AbortController();
        if (!user) return;
        setLoading(true);
        setError("");
        void listBusinessObjects(search, false, offset, controller.signal)
            .then((rows) => {
                if (!controller.signal.aborted) setRows(rows);
            })
            .catch((e) => {
                if (!controller.signal.aborted) setError(e.message);
            })
            .finally(() => {
                if (!controller.signal.aborted) setLoading(false);
            });
        return () => controller.abort();
    }, [user, search, offset, refresh]);
    useEffect(() => {
        const controller = new AbortController();
        setView(undefined);
        if (ref && user)
            void readBusinessObject(ref, controller.signal)
                .then((v) => {
                    if (!controller.signal.aborted) setView(v);
                })
                .catch((e) => {
                    if (!controller.signal.aborted) setError(e.message);
                });
        return () => controller.abort();
    }, [user, ref?.objectId, ref?.version]);
    return (
        <div className="space-y-2 rounded-lg border border-border p-3">
            <div className="flex gap-2">
                <Input aria-label="检索品牌" value={query} maxLength={120} disabled={disabled} onChange={(e) => setQuery(e.target.value)} />
                <Button
                    disabled={disabled || loading}
                    onClick={() => {
                        setOffset(0);
                        setSearch(query.trim());
                        setRefresh((n) => n + 1);
                    }}
                >
                    检索品牌
                </Button>
                <Link to="/business-objects" target="_blank" rel="noopener noreferrer">
                    管理品牌资料
                </Link>
            </div>
            <Select
                className="w-full"
                aria-label="选择品牌版本"
                disabled={disabled || loading}
                value={ref ? `${ref.objectId}:${ref.version}` : undefined}
                placeholder="选择当前账号的品牌"
                options={rows.map((r) => ({ value: `${r.objectId}:${r.version}`, label: `${r.title} · v${r.version}` }))}
                onChange={(key) => {
                    const row = rows.find((r) => `${r.objectId}:${r.version}` === key);
                    if (row) onChange({ objectId: row.objectId, version: row.version, type: "brand", schemaVersion: 1 });
                }}
            />
            <div className="flex gap-2">
                <Button size="small" disabled={disabled || loading || offset === 0} onClick={() => setOffset((n) => Math.max(0, n - 30))}>
                    上一页
                </Button>
                <Button size="small" disabled={disabled || loading || rows.length < 30} onClick={() => setOffset((n) => n + 30)}>
                    下一页
                </Button>
            </div>
            {error ? (
                <p role="alert" className="text-destructive">
                    {error}
                </p>
            ) : null}
            {view ? (
                <div className="space-y-1 text-sm">
                    <p>
                        {view.brand.name} · 固定版本 {view.reference.version}
                        {view.archived ? "（已归档，不能发起新调用）" : ""}
                    </p>
                    <p>受众：{view.brand.audience}</p>
                    <p>定位：{view.brand.positioning}</p>
                    <p>已选择版本不会随品牌更新自动变化。</p>
                </div>
            ) : null}
        </div>
    );
}
