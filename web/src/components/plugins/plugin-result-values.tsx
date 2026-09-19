// Shared by inline inspection, run receipts and canvas nodes; uses semantic text/border tokens.
import type { PluginResultView } from "@/services/api/plugin-operations";
import { pluginResultField, pluginResultFieldText } from "@/lib/plugins/plugin-result-view";

export function PluginResultValues({ value, view }: { value: unknown; view?: PluginResultView }) {
    if (!view || view.component !== "key-value/v1") return <pre className="max-h-48 overflow-auto whitespace-pre-wrap break-all text-xs">{JSON.stringify(value, null, 2)}</pre>;
    return <dl className="space-y-2 text-sm" data-slot="plugin-result-values">
        {view.fields.map((field, index) => <div key={`${field.path}:${index}`} className="grid grid-cols-2 gap-2 border-b border-border pb-1">
            <dt className="text-muted-foreground">{field.label}</dt>
            <dd className="break-all whitespace-pre-wrap">{pluginResultFieldText(pluginResultField(value, field.path))}</dd>
        </div>)}
    </dl>;
}
