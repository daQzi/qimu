import { App, Button, Checkbox, Select } from "antd";
import { useCallback, useEffect, useRef, useState } from "react";
import { useUserStore } from "@/stores/use-user-store";
import { activateApplicationPlugin, fetchApplicationPlugins, manageApplicationPlugin, type ApplicationPlugin } from "@/services/api/application-plugins";

const reasonLabels: Record<string, string> = {
    plugin_disabled: "尚未启用或已由平台停用",
    plugin_revoked: "所选版本已撤回",
    plugin_dependency_missing: "请先启用所需依赖版本，再重新启用此应用",
    scope_forbidden: "尚未授予所需权限",
    operation_unavailable: "操作执行将在后续阶段开放",
};

export function ApplicationPluginsPanel({ admin = false, refreshToken = 0 }: { admin?: boolean; refreshToken?: number }) {
    const { message, modal } = App.useApp();
    const userID = useUserStore((s) => s.user?.id);
    const [items, setItems] = useState<ApplicationPlugin[]>([]);
    const [loadedFor, setLoadedFor] = useState<string>();
    const visibleItems = loadedFor === userID ? items : [];
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState("");
    const [saving, setSaving] = useState("");
    const [refresh, setRefresh] = useState(0);
    const generation = useRef(0);
    const [selection, setSelection] = useState<Record<string, { releaseId: string; grants: string[] }>>({});
    const reload = useCallback(() => setRefresh((v) => v + 1), []);

    useEffect(() => {
        const current = ++generation.current;
        const controller = new AbortController();
        setItems([]);
        setLoadedFor(undefined);
        setSelection({});
        setError("");
        setSaving("");
        if (!userID) return () => controller.abort();
        setLoading(true);
        void fetchApplicationPlugins(controller.signal)
            .then((result) => {
                if (current !== generation.current || useUserStore.getState().user?.id !== userID) return;
                setItems(result.applications);
                setLoadedFor(userID);
                setSelection(
                    Object.fromEntries(
                        result.applications.map((item) => [
                            item.id,
                            {
                                releaseId: item.state.installedReleaseId || item.releases.find((r) => !r.revoked)?.id || item.releases[0]?.id || "",
                                grants: item.state.grantedPermissions || [],
                            },
                        ]),
                    ),
                );
            })
            .catch((cause) => {
                if (!controller.signal.aborted && current === generation.current && useUserStore.getState().user?.id === userID) setError(cause instanceof Error ? cause.message : "读取应用插件失败");
            })
            .finally(() => {
                if (current === generation.current) setLoading(false);
            });
        return () => {
            controller.abort();
            generation.current++;
        };
    }, [userID, refresh, refreshToken]);

    const mutate = async (id: string, action: () => Promise<unknown>) => {
        const current = generation.current;
        setSaving(id);
        try {
            await action();
            if (current !== generation.current || useUserStore.getState().user?.id !== userID) return;
            message.success("应用插件状态已保存");
            reload();
        } catch (cause) {
            if (current === generation.current && useUserStore.getState().user?.id === userID) {
                setError(cause instanceof Error ? cause.message : "保存失败，请刷新重试");
                setSaving("");
            }
        }
    };

    return (
        <section aria-label="应用插件版本目录" className="my-4 space-y-3 rounded-lg border border-border bg-card p-4">
            <div className="flex items-center justify-between gap-3">
                <h2 className="font-medium">应用插件 · 版本与技能</h2>
                <Button loading={loading} onClick={reload}>
                    刷新应用
                </Button>
            </div>
            <p className="text-sm text-muted-foreground">启用后，获准使用的技能会进入“我的技能”。操作清单可查阅，执行能力尚未开放。</p>
            {error && (
                <p role="alert" className="text-sm text-destructive">
                    {error}
                </p>
            )}
            {!loading && !error && !visibleItems.length && <p className="text-sm text-muted-foreground">暂无应用插件，由管理员上传 v3 应用包后可在此选择版本。</p>}
            {visibleItems.map((item) => {
                const selected = selection[item.id];
                const release = item.releases.find((r) => r.id === selected?.releaseId);
                if (!release || !selected) return null;
                const disabled = !!saving || loading;
                return (
                    <article key={item.id} className="space-y-3 rounded-lg border border-border p-4" aria-label={release.manifest.name}>
                        <div className="flex flex-wrap items-center justify-between gap-2">
                            <div>
                                <h3 className="font-medium">{release.manifest.name}</h3>
                                <p className="text-xs text-muted-foreground">
                                    {item.id} · 发布者 {item.publisherId}
                                </p>
                            </div>
                            <span className="text-sm">{!item.installed ? "已卸载，历史版本保留" : item.state.effectiveEnabled ? "已启用" : item.state.enabled ? "已启用但当前不可用" : "未启用"}</span>
                        </div>
                        <p className="text-sm">{release.manifest.description}</p>
                        <label className="flex flex-wrap items-center gap-2">
                            <span>发布版本</span>
                            <Select
                                aria-label={`${item.id} 发布版本`}
                                className="min-w-48"
                                value={selected.releaseId}
                                disabled={disabled}
                                options={item.releases.map((r) => ({ value: r.id, label: `${r.version}${r.revoked ? "（已撤回）" : ""}` }))}
                                onChange={(releaseId) => setSelection((v) => ({ ...v, [item.id]: { releaseId, grants: [] } }))}
                            />
                        </label>
                        <p className="break-all text-xs text-muted-foreground">内容摘要：{release.digest}</p>
                        <fieldset className="space-y-2">
                            <legend className="text-sm">本次授予的权限</legend>
                            {release.manifest.permissions.length ? (
                                <Checkbox.Group value={selected.grants} disabled={disabled} options={release.manifest.permissions} onChange={(values) => setSelection((v) => ({ ...v, [item.id]: { ...selected, grants: values as string[] } }))} />
                            ) : (
                                <span className="text-sm text-muted-foreground">此插件未申请操作权限</span>
                            )}
                        </fieldset>
                        {item.state.reason && <p className="text-sm text-muted-foreground">{reasonLabels[item.state.reason] || item.state.reason}</p>}
                        {!!release.dependencyLock.length && <p className="text-sm text-muted-foreground">固定依赖版本：{release.dependencyLock.join("、")}</p>}
                        <div className="flex flex-wrap gap-2">
                            <Button
                                type="primary"
                                disabled={disabled || !item.installed || !item.available || release.revoked}
                                loading={saving === item.id}
                                onClick={() => void mutate(item.id, () => activateApplicationPlugin(item.id, { releaseId: release.id, enabled: true, grantedPermissions: selected.grants, revision: item.state.revision }))}
                            >
                                保存版本与授权并启用
                            </Button>
                            <Button
                                disabled={disabled || !item.state.enabled}
                                onClick={() => void mutate(item.id, () => activateApplicationPlugin(item.id, { releaseId: item.state.installedReleaseId, enabled: false, grantedPermissions: item.state.grantedPermissions, revision: item.state.revision }))}
                            >
                                停用我的插件
                            </Button>
                            {admin && (
                                <>
                                    <Button disabled={disabled || !item.installed} onClick={() => void mutate(item.id, () => manageApplicationPlugin(item.id, { action: "availability", available: !item.available, revision: item.revision }))}>
                                        {item.available ? "平台停用" : "平台开放"}
                                    </Button>
                                    <Button
                                        danger
                                        disabled={disabled || release.revoked}
                                        onClick={() =>
                                            modal.confirm({
                                                title: `撤回 ${release.manifest.name} ${release.version}？`,
                                                content: "该版本将禁止重新启用，技能和历史文件保留。需要修复时请发布新版本。",
                                                onOk: () => mutate(item.id, () => manageApplicationPlugin(item.id, { action: "revoke", releaseId: release.id, revision: item.revision })),
                                            })
                                        }
                                    >
                                        撤回所选版本
                                    </Button>
                                    <Button
                                        danger
                                        disabled={disabled || !item.installed}
                                        onClick={() =>
                                            modal.confirm({
                                                title: `卸载 ${release.manifest.name}？`,
                                                content: "将关闭新的启用入口，保留发布版本、技能和历史引用。",
                                                onOk: () => mutate(item.id, () => manageApplicationPlugin(item.id, { action: "uninstall", revision: item.revision })),
                                            })
                                        }
                                    >
                                        卸载应用
                                    </Button>
                                </>
                            )}
                        </div>
                        <details>
                            <summary className="cursor-pointer text-sm">技能与操作清单</summary>
                            <ul className="mt-2 space-y-1 text-sm">
                                {release.skills.map((skill) => (
                                    <li key={skill.id}>
                                        技能：{skill.localSkillId} · {skill.skillId}
                                    </li>
                                ))}
                                {release.operations.map((op) => (
                                    <li key={op.id}>
                                        操作：{item.id}.{op.id} — {reasonLabels[op.reason] || op.reason}
                                    </li>
                                ))}
                            </ul>
                        </details>
                    </article>
                );
            })}
        </section>
    );
}
