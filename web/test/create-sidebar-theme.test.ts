import { describe, expect, test } from "bun:test";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";

import { DEFAULT_CLASSIC_SKIN, duplicateSkinDefinition } from "../src/lib/skin-themes";
import { getCanvasTheme } from "../src/lib/canvas-theme";

describe("create page sidebar theme", () => {
    test("direct workspace routes use the same rail as returning home", () => {
        const shell = readFileSync(resolve(import.meta.dir, "../src/components/layout/app-top-nav.tsx"), "utf8");
        expect(shell).toContain("const creationWorkspace = !hideChrome;");
        expect(shell).not.toContain('const creationWorkspace = pathname === "/"');
    });
    test("uses a compact stacked discovery rail with neutral navigation states", () => {
        const styles = readFileSync(resolve(import.meta.dir, "../src/styles/globals.css"), "utf8");

        expect(styles).toContain("--size-discovery-rail: 108px");
        expect(styles).toContain("--discovery-sidebar-foreground");
        expect(styles).toContain(".app-workspace-sidebar-nav:not(.is-collapsed) .app-workspace-nav-link");
        expect(styles).toContain("flex-direction: column");
        expect(styles).toContain(".app-workspace-sidebar-nav:not(.is-collapsed) .app-workspace-nav-main");
    });

    test("derives canvas surfaces and controls from the active skin", () => {
        const custom = duplicateSkinDefinition(DEFAULT_CLASSIC_SKIN, ["classic"]);
        custom.tokens.light.canvas = "#f4eadf";
        custom.tokens.light.surface = "#fffaf4";
        custom.tokens.light.surfaceRaised = "#ffffff";
        custom.tokens.light.text = "#2f241c";
        custom.tokens.light.textMuted = "#75675d";
        custom.tokens.light.border = "#dbcfc3";
        custom.tokens.light.primary = "#9a5b3d";
        custom.tokens.light.primaryForeground = "#ffffff";
        custom.tokens.light.selected = "#ead7ca";
        custom.tokens.light.selectedForeground = "#5b3020";

        const theme = getCanvasTheme("light", custom);

        expect(theme.canvas.background).toBe("#f4eadf");
        expect(theme.node.fill).toBe("#fffaf4");
        expect(theme.node.text).toBe("#2f241c");
        expect(theme.toolbar.panel).toContain("255,255,255");
        expect(theme.accent.primary).toBe("#9a5b3d");
        expect(theme.node.activeStroke).toBe("#ead7ca");
    });

    test("canvas entry points subscribe to skin changes", async () => {
        const [projectSource, canvasNodeSource, topBarSource, infiniteCanvasSource] = await Promise.all([
            Bun.file(new URL("../src/pages/canvas/project.tsx", import.meta.url)).text(),
            Bun.file(new URL("../src/components/canvas/canvas-node.tsx", import.meta.url)).text(),
            Bun.file(new URL("../src/pages/canvas/canvas-project-top-bar.tsx", import.meta.url)).text(),
            Bun.file(new URL("../src/components/canvas/infinite-canvas.tsx", import.meta.url)).text(),
        ]);

        for (const source of [projectSource, canvasNodeSource, topBarSource, infiniteCanvasSource]) {
            expect(source).toContain("useCanvasTheme");
        }
        expect(projectSource).not.toContain("const theme = canvasThemes[colorTheme]");
    });

    test("scopes the compact rail to the creation workspace", () => {
        const styles = readFileSync(resolve(import.meta.dir, "../src/styles/globals.css"), "utf8");

        expect(styles).toContain(".app-workspace-shell.is-creation-workspace .app-workspace-sidebar {");
        expect(styles).toContain(".app-workspace-shell.is-creation-workspace .app-workspace-stage");
        expect(styles).toContain("background: color-mix(in srgb, var(--workspace-sidebar) 96%, var(--foreground) 4%);");
        expect(styles.match(/--workspace-sidebar: var\(--workspace-navigation\);/g)?.length).toBeGreaterThanOrEqual(4);
    });
});
