import { useMemo } from "react";

import { getCanvasTheme, type CanvasColorTheme } from "@/lib/canvas-theme";
import { DEFAULT_CLASSIC_SKIN } from "@/lib/skin-themes";
import { useAppearanceStore } from "@/stores/use-appearance-store";
import { useThemeStore } from "@/stores/use-theme-store";

export function useCanvasTheme(colorTheme?: CanvasColorTheme) {
    const currentColorTheme = useThemeStore((state) => state.theme);
    const skin = useAppearanceStore((state) => state.appearance.activeSkin);
    const resolvedColorTheme = colorTheme ?? currentColorTheme;
    return useMemo(() => getCanvasTheme(resolvedColorTheme, skin || DEFAULT_CLASSIC_SKIN), [resolvedColorTheme, skin]);
}
