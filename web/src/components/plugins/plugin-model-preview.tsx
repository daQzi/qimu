export function PluginModelPreview({ value }: { value: unknown }) {
    const preview = value && typeof value === "object" ? (value as Record<string, unknown>) : {};
    const amount = typeof preview.amountMicrocredits === "number" ? preview.amountMicrocredits / 1_000_000 : undefined;
    const resources = preview.resources && typeof preview.resources === "object" ? Object.values(preview.resources) : [];
    return (
        <div className="space-y-2 text-sm">
            <p>系统模型：{typeof preview.model === "string" ? preview.model : "待确认"}</p>
            <p>渠道：{typeof preview.channel === "string" ? preview.channel : "待确认"} · {typeof preview.targetHost === "string" ? preview.targetHost : "目标待确认"}</p>
            <p>
                本次授权额度：{amount === undefined ? "报价不可用" : `${amount} 积分`}
                {preview.estimated === true ? "（按 Token 预估，实际结算受此上限约束）" : ""}。
            </p>
            <p>{resources.length ? `将向该模型渠道发送 ${resources.length} 个素材。` : "本次发送文本内容，不发送媒体文件。"}调用内容包含插件提示要求、输入和输出格式。</p>
            {resources.length > 0 && (
                <ul className="list-inside list-disc">
                    {resources.map((item, index) => {
                        const resource = item && typeof item === "object" ? (item as Record<string, unknown>) : {};
                        return <li key={index}>{typeof resource.resourceId === "string" ? resource.resourceId : "资源信息不可用"}</li>;
                    })}
                </ul>
            )}
            <p className="text-muted-foreground">模型已返回但结果校验失败时仍可能产生费用；不会自动重试或切换付费模型。视频音轨尚未核验，本期不生成成片。</p>
        </div>
    );
}
