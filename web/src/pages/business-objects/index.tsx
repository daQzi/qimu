import { useEffect, useRef, useState } from "react";
import { Button, Input, Select } from "antd";
import { Link } from "react-router";
import { useUserStore } from "@/stores/use-user-store";
import { archiveBusinessObject, listBusinessObjects, readBusinessObject, saveBusinessObject, type Brand, type BusinessObject, type ObjectReference, type ObjectView, type ObjectWrite } from "@/services/api/business-objects";
import { loadObjectWrite, saveObjectWrite, clearObjectWrite } from "@/services/business-object-pending";

const emptyBrand = (): Brand => ({ name: "", audience: "", positioning: "", claims: [], restrictions: [] });
export default function BusinessObjectsPage() {
    const user = useUserStore((s) => s.user?.id);
    return user ? <OwnedPage key={user} user={user} /> : null;
}
function OwnedPage({ user }: { user: string }) {
    const [rows, setRows] = useState<BusinessObject[]>([]);
    const [query, setQuery] = useState("");
    const [archived, setArchived] = useState(false);
    const [offset, setOffset] = useState(0);
    const [refresh, setRefresh] = useState(0);
    const [brand, setBrand] = useState<Brand>(emptyBrand);
    const [selected, setSelected] = useState<ObjectView>();
    const [source, setSource] = useState<ObjectReference>();
    const [pending, setPending] = useState<ObjectWrite>();
    const [ready, setReady] = useState(false);
    const [recovery, setRecovery] = useState(false);
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState("");
    const [notice, setNotice] = useState("");
    const active = useRef(true);
    const lock = useRef(false);
    const owned = () => active.current && useUserStore.getState().user?.id === user;
    useEffect(() => {
        active.current = true;
        void loadObjectWrite(user)
            .then((v) => {
                if (owned()) {
                    if (v) {
                        setPending(v);
                        setBrand(v.brand);
                    }
                    setReady(true);
                }
            })
            .catch((e) => {
                if (owned()) {
                    setError(e.message);
                    setRecovery(true);
                }
            });
        return () => {
            active.current = false;
        };
    }, [user]);
    useEffect(() => {
        const controller = new AbortController();
        void listBusinessObjects(query.trim(), archived, offset, controller.signal)
            .then((v) => {
                if (!controller.signal.aborted && owned()) setRows(v);
            })
            .catch((e) => {
                if (!controller.signal.aborted && owned()) setError(e.message);
            });
        return () => controller.abort();
    }, [query, archived, offset, refresh]);
    const perform = async (fn: () => Promise<void>) => {
        if (lock.current) return;
        lock.current = true;
        setBusy(true);
        setError("");
        setNotice("");
        try {
            await fn();
        } catch (e) {
            if (owned()) setError(e instanceof Error ? e.message : String(e));
        } finally {
            lock.current = false;
            if (owned()) setBusy(false);
        }
    };
    const open = (ref: ObjectReference) =>
        perform(async () => {
            const v = await readBusinessObject(ref);
            if (owned()) {
                setSelected(v);
                setSource(undefined);
                setBrand(v.brand);
            }
        });
    const save = () =>
        perform(async () => {
            const request = pending || {
                ...(selected ? { objectId: selected.reference.objectId } : {}),
                expectedVersion: selected?.reference.version || 0,
                clientKey: crypto.randomUUID(),
                brand: {
                    ...brand,
                    name: brand.name.trim(),
                    audience: brand.audience.trim(),
                    positioning: brand.positioning.trim(),
                    claims: brand.claims.map((v) => v.trim()).filter(Boolean),
                    restrictions: brand.restrictions.map((v) => v.trim()).filter(Boolean),
                },
                ...(source ? { source } : {}),
            };
            if (!pending) {
                await saveObjectWrite(user, request);
                if (!owned()) return;
                setPending(request);
            }
            const v = await saveBusinessObject(request);
            await clearObjectWrite(user);
            if (owned()) {
                setPending(undefined);
                setSelected(v);
                setSource(undefined);
                setBrand(v.brand);
                setRefresh((n) => n + 1);
                setNotice("已保存到服务端；历史版本保持不变");
            }
        });
    const disabled = busy || !ready || !!pending;
    return (
        <main className="mx-auto max-w-5xl space-y-4 p-6">
            <div className="flex gap-4">
                <h1 className="text-xl font-semibold">品牌资料</h1>
                <Link to="/workbenches">插件工作台</Link>
            </div>
            <p>品牌资料可被多个插件引用。修改生成新版本，归档保留历史；本阶段支持文本资料，不复制媒体。</p>
            {error ? (
                <p role="alert" className="text-destructive">
                    {error}
                </p>
            ) : null}
            {notice ? <p role="status">{notice}</p> : null}
            <div className="flex flex-wrap gap-2">
                <Input
                    aria-label="品牌名称检索"
                    className="max-w-xs"
                    value={query}
                    maxLength={120}
                    onChange={(e) => {
                        setQuery(e.target.value);
                        setOffset(0);
                    }}
                />
                <Select
                    aria-label="品牌状态"
                    value={archived ? "archived" : "active"}
                    options={[
                        { value: "active", label: "使用中" },
                        { value: "archived", label: "已归档" },
                    ]}
                    onChange={(v) => {
                        setArchived(v === "archived");
                        setOffset(0);
                    }}
                />
                <Button onClick={() => setRefresh((n) => n + 1)}>刷新列表</Button>
                <Button
                    disabled={disabled}
                    onClick={() => {
                        setSelected(undefined);
                        setSource(undefined);
                        setBrand(emptyBrand());
                    }}
                >
                    新建品牌
                </Button>
            </div>
            <div className="grid gap-4 md:grid-cols-[240px_1fr]">
                <aside className="space-y-2">
                    {rows.length ? (
                        rows.map((r) => (
                            <Button key={r.objectId} block disabled={disabled} onClick={() => void open({ objectId: r.objectId, version: r.version, type: "brand", schemaVersion: 1 })}>
                                {r.title} · v{r.version}
                            </Button>
                        ))
                    ) : (
                        <p>暂无品牌资料。</p>
                    )}
                    <Button disabled={offset === 0} onClick={() => setOffset((n) => Math.max(0, n - 30))}>
                        上一页
                    </Button>
                    <Button disabled={rows.length < 30} onClick={() => setOffset((n) => n + 30)}>
                        下一页
                    </Button>
                </aside>
                <section className="space-y-3 rounded-lg border border-border bg-card p-4">
                    {selected ? (
                        <div className="space-y-2">
                            <p className="break-all">{selected.reference.objectId}</p>
                            <Select
                                aria-label="查看历史版本"
                                value={selected.reference.version}
                                disabled={disabled}
                                options={Array.from({ length: selected.currentVersion }, (_, i) => ({ value: i + 1, label: `版本 ${i + 1}` }))}
                                onChange={(version) => void open({ ...selected.reference, version })}
                            />
                            <Button
                                disabled={disabled}
                                onClick={() => {
                                    setSource(selected.reference);
                                    setSelected(undefined);
                                }}
                            >
                                基于此版本另建品牌
                            </Button>
                            <Button
                                disabled={disabled}
                                onClick={() =>
                                    void perform(async () => {
                                        await archiveBusinessObject(selected.reference.objectId, selected.currentVersion, !selected.archived);
                                        if (owned()) {
                                            setSelected({ ...selected, archived: !selected.archived });
                                            setRefresh((n) => n + 1);
                                        }
                                    })
                                }
                            >
                                {selected.archived ? "恢复使用" : "归档品牌"}
                            </Button>
                        </div>
                    ) : null}
                    {source ? (
                        <p>
                            派生自 {source.objectId} · v{source.version}，保存将创建独立品牌。
                        </p>
                    ) : null}
                    {(
                        [
                            ["name", "品牌名称", 160],
                            ["audience", "目标受众", 1000],
                            ["positioning", "品牌定位", 2000],
                        ] as const
                    ).map(([field, label, max]) => (
                        <label key={field} className="block space-y-1">
                            <span>{label} *</span>
                            <Input.TextArea aria-label={label} disabled={disabled || selected?.archived} value={brand[field]} maxLength={max} onChange={(e) => setBrand({ ...brand, [field]: e.target.value })} />
                        </label>
                    ))}
                    {(
                        [
                            ["claims", "已确认卖点"],
                            ["restrictions", "表达限制"],
                        ] as const
                    ).map(([field, label]) => (
                        <label key={field} className="block space-y-1">
                            <span>{label}（每行一项，最多 20 项）</span>
                            <Input.TextArea aria-label={label} rows={4} disabled={disabled || selected?.archived} value={brand[field].join("\n")} onChange={(e) => setBrand({ ...brand, [field]: e.target.value.split("\n") })} />
                        </label>
                    ))}
                    <Button type="primary" disabled={busy || !ready || (!pending && (!brand.name.trim() || !brand.audience.trim() || !brand.positioning.trim() || !!selected?.archived))} onClick={() => void save()}>
                        {pending ? "原样重试保存" : "保存新版本"}
                    </Button>
                    {pending || recovery ? (
                        <div>
                            <p>上次保存结果待确认，请原样重试。核对品牌列表后，才可放弃此重试记录。</p>
                            <Button
                                disabled={busy}
                                onClick={() =>
                                    void perform(async () => {
                                        await clearObjectWrite(user);
                                        if (owned()) {
                                            setReady(true);
                                            setRecovery(false);
                                            setPending(undefined);
                                            setSelected(undefined);
                                            setSource(undefined);
                                            setBrand(emptyBrand());
                                            setNotice("已解除锁定，请先核对列表，避免重复新建");
                                        }
                                    })
                                }
                            >
                                已核对列表，解除锁定
                            </Button>
                        </div>
                    ) : null}
                </section>
            </div>
        </main>
    );
}
