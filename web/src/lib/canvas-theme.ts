import { DEFAULT_CLASSIC_SKIN, type SkinDefinition, type SkinModeTokens } from "@/lib/skin-themes";

export type CanvasColorTheme = "light" | "dark";
export type CanvasBackgroundMode = "dots" | "lines" | "blank";

export type CanvasThemeStyle = {
    kind: "classic";
    gridPattern: "dots" | "lines";
    selectionMode: "outline";
    runningMode: "progress";
};

function rgbaFromHex(value: string, alpha: number) {
    const normalized = value.replace("#", "");
    const red = Number.parseInt(normalized.slice(0, 2), 16);
    const green = Number.parseInt(normalized.slice(2, 4), 16);
    const blue = Number.parseInt(normalized.slice(4, 6), 16);
    return `rgba(${red},${green},${blue},${alpha})`;
}

function shadowValues(mode: SkinModeTokens, dark: boolean, style: SkinDefinition["tokens"]["components"]["shadowStyle"]) {
    if (style === "none") return { node: "none", nodeHover: "none", spatial: "none" };
    const color = dark ? "rgba(0,0,0,.6)" : rgbaFromHex(mode.text, 0.18);
    if (style === "strong") {
        return {
            node: `0 8px 24px ${color}`,
            nodeHover: `0 14px 34px ${color}`,
            spatial: `0 24px 72px ${color}`,
        };
    }
    return {
        node: `0 6px 18px ${color}`,
        nodeHover: `0 10px 24px ${color}`,
        spatial: `0 18px 54px ${color}`,
    };
}

export function getCanvasTheme(colorTheme: CanvasColorTheme, skin: SkinDefinition = DEFAULT_CLASSIC_SKIN) {
    if (skin.id === DEFAULT_CLASSIC_SKIN.id) return CLASSIC_CANVAS_THEMES[colorTheme];

    const dark = colorTheme === "dark";
    const palette = skin.tokens[colorTheme];
    const shadows = shadowValues(palette, dark, skin.tokens.components.shadowStyle);
    const grid = rgbaFromHex(palette.workspaceGrid, 0.8);
    const selected = rgbaFromHex(palette.selected, dark ? 0.72 : 0.78);
    const selectedSoft = rgbaFromHex(palette.selected, dark ? 0.22 : 0.34);
    const surface = rgbaFromHex(palette.surface, dark ? 0.82 : 0.72);
    const raised = rgbaFromHex(palette.surfaceRaised, dark ? 0.97 : 0.94);

    return {
        style: {
            kind: "classic" as const,
            gridPattern: "dots" as const,
            selectionMode: "outline" as const,
            runningMode: "progress" as const,
        },
        canvas: {
            background: palette.canvas,
            dot: grid,
            line: grid,
            selectionFill: selectedSoft,
        },
        node: {
            label: palette.textMuted,
            fill: palette.surface,
            panel: palette.surfaceRaised,
            stroke: palette.border,
            edge: rgbaFromHex(palette.controlBorder, dark ? 0.8 : 0.65),
            shadow: shadows.node,
            hoverShadow: shadows.nodeHover,
            activeStroke: palette.selected,
            placeholder: palette.controlDisabledForeground,
            text: palette.text,
            muted: palette.textMuted,
            faint: palette.controlDisabledForeground,
        },
        frame: {
            fill: rgbaFromHex(palette.selected, dark ? 0.08 : 0.12),
            stroke: rgbaFromHex(palette.selected, dark ? 0.55 : 0.72),
            activeFill: selectedSoft,
            activeStroke: palette.selected,
            preview: raised,
        },
        toolbar: {
            panel: raised,
            border: palette.border,
            item: palette.textMuted,
            itemHover: rgbaFromHex(palette.controlHover, dark ? 0.85 : 0.9),
            activeBg: selected,
            activeText: palette.selectedForeground,
        },
        spatial: {
            surface,
            elevated: raised,
            dropzone: rgbaFromHex(palette.canvas, dark ? 0.9 : 0.78),
            glow: rgbaFromHex(palette.controlFocus, dark ? 0.24 : 0.18),
            glowStrong: rgbaFromHex(palette.selected, dark ? 0.58 : 0.52),
            shadow: dark ? "rgba(0,0,0,.6)" : rgbaFromHex(palette.text, 0.18),
        },
        timeline: {
            trackFill: palette.surfaceRaised,
            trackBorder: rgbaFromHex(palette.border, 0.85),
            clipVideo: rgbaFromHex(palette.info, dark ? 0.22 : 0.16),
            clipAudio: rgbaFromHex(palette.success, dark ? 0.2 : 0.14),
            clipSubtitle: rgbaFromHex(palette.warning, dark ? 0.2 : 0.16),
            clipSelectedBorder: palette.selected,
            handle: rgbaFromHex(palette.surface, dark ? 0.5 : 0.72),
            rulerTick: rgbaFromHex(palette.textMuted, dark ? 0.62 : 0.55),
            rulerLabel: palette.textMuted,
            playhead: palette.primary,
            entryActive: selectedSoft,
            entryHover: rgbaFromHex(palette.controlHover, dark ? 0.8 : 0.9),
        },
        accent: {
            primary: palette.primary,
            primarySoft: selectedSoft,
            onPrimary: palette.primaryForeground,
            danger: palette.danger,
        },
    } as const;
}

const CLASSIC_CANVAS_THEMES = {
    light: {
        style: { kind: "classic" as const, gridPattern: "dots" as const, selectionMode: "outline" as const, runningMode: "progress" as const },
        canvas: { background: "#f0f0f0", dot: "rgba(0,0,0,.80)", line: "rgba(0,0,0,.80)", selectionFill: "rgba(17,17,17,.10)" },
        node: {
            label: "#4b5563", fill: "#ffffff", panel: "#ffffff", stroke: "#e2e4e8", edge: "rgba(15,23,42,.16)",
            shadow: "0 6px 18px rgba(15,23,42,.08)", hoverShadow: "0 10px 24px rgba(15,23,42,.12)", activeStroke: "#111827",
            placeholder: "#9ca3af", text: "#111827", muted: "#6b7280", faint: "#9ca3af",
        },
        frame: { fill: "rgba(17,24,39,.025)", stroke: "rgba(17,24,39,.18)", activeFill: "rgba(17,17,17,.05)", activeStroke: "#171717", preview: "rgba(255,255,255,.82)" },
        toolbar: { panel: "rgba(255,255,255,.94)", border: "rgba(17,24,39,.10)", item: "#4b5563", itemHover: "rgba(17,24,39,.06)", activeBg: "rgba(17,24,39,.10)", activeText: "#111827" },
        spatial: { surface: "rgba(255,255,255,.72)", elevated: "rgba(255,255,255,.94)", dropzone: "rgba(248,250,252,.78)", glow: "rgba(17,17,17,.14)", glowStrong: "rgba(17,17,17,.42)", shadow: "rgba(15,23,42,.18)" },
        timeline: {
            trackFill: "#f6f8fb", trackBorder: "rgba(17,24,39,.08)", clipVideo: "rgba(79,110,232,.16)", clipAudio: "rgba(16,185,129,.14)", clipSubtitle: "rgba(245,158,11,.16)",
            clipSelectedBorder: "#171717", handle: "rgba(255,255,255,.72)", rulerTick: "rgba(17,24,39,.28)", rulerLabel: "#6b7280", playhead: "#171717", entryActive: "rgba(17,17,17,.10)", entryHover: "rgba(17,24,39,.05)",
        },
        accent: { primary: "#171717", primarySoft: "rgba(17,17,17,.10)", onPrimary: "#ffffff", danger: "#f87171" },
    },
    dark: {
        style: { kind: "classic" as const, gridPattern: "dots" as const, selectionMode: "outline" as const, runningMode: "progress" as const },
        canvas: { background: "#000000", dot: "rgba(175,175,175,.80)", line: "rgba(175,175,175,.80)", selectionFill: "rgba(255,255,255,.12)" },
        node: {
            label: "#a3a3a3", fill: "#181818", panel: "#141414", stroke: "rgba(255,255,255,.12)", edge: "rgba(255,255,255,.18)",
            shadow: "0 8px 24px rgba(0,0,0,.34)", hoverShadow: "0 12px 30px rgba(0,0,0,.46)", activeStroke: "#f1f1f1",
            placeholder: "#737373", text: "#ededed", muted: "#a3a3a3", faint: "#666666",
        },
        frame: { fill: "rgba(255,255,255,.025)", stroke: "rgba(190,198,210,.15)", activeFill: "rgba(255,255,255,.08)", activeStroke: "#f5f5f5", preview: "rgba(20,20,20,.94)" },
        toolbar: { panel: "rgba(20,20,20,.97)", border: "rgba(255,255,255,.1)", item: "#d4d4d4", itemHover: "rgba(255,255,255,.07)", activeBg: "rgba(255,255,255,.10)", activeText: "#f5f6f8" },
        spatial: { surface: "rgba(22,22,22,.82)", elevated: "rgba(15,15,15,.97)", dropzone: "rgba(8,8,8,.9)", glow: "rgba(255,255,255,.12)", glowStrong: "rgba(255,255,255,.36)", shadow: "rgba(0,0,0,.6)" },
        timeline: {
            trackFill: "#101114", trackBorder: "rgba(255,255,255,.08)", clipVideo: "rgba(96,126,234,.22)", clipAudio: "rgba(16,185,129,.20)", clipSubtitle: "rgba(245,158,11,.20)",
            clipSelectedBorder: "#f5f5f5", handle: "rgba(255,255,255,.5)", rulerTick: "rgba(255,255,255,.26)", rulerLabel: "#a3a3a3", playhead: "#f5f5f5", entryActive: "rgba(255,255,255,.10)", entryHover: "rgba(255,255,255,.06)",
        },
        accent: { primary: "#f5f5f5", primarySoft: "rgba(255,255,255,.11)", onPrimary: "#131313", danger: "#fb7185" },
    },
} as const;

export type CanvasTheme = ReturnType<typeof getCanvasTheme>;

// 兼容不需要 React 状态的预览、算法和旧调用方；有状态组件使用 useCanvasTheme。
export const canvasThemes: Record<CanvasColorTheme, CanvasTheme> = {
    light: getCanvasTheme("light"),
    dark: getCanvasTheme("dark"),
};
