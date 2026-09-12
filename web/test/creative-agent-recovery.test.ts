import { expect, test } from "bun:test";
import { recoverCreativeResponse } from "@/services/creative-agent-recovery";
import { parseToolArguments } from "@/components/canvas/canvas-assistant-online-tools";
import { initialCreativeState } from "@/lib/creation/creative-agent-state";
import { defaultConfig } from "@/stores/use-config-store";

const question = { field: "custom.mood", type: "text", title: "希望是什么氛围？" };
const context = { config: defaultConfig, state: initialCreativeState(), latestInput: "帮我创作短片" };
const reply = (input: unknown) => ({ content: "", toolCalls: [{ id: "creative-call", type: "function" as const, function: { name: "creative_respond", arguments: JSON.stringify(input) } }] });

test("根问题数组通过恢复后，画布对象解析入口可直接展示问题", async () => {
    const result = await recoverCreativeResponse(reply([question]), [], context,
        async () => { throw new Error("有效问题无需模型修复"); },
        () => { throw new Error("有效问题不应进入修复流程"); });
    const input = parseToolArguments(result.toolCalls[0].function.arguments);
    expect(input.questions).toEqual([question]);
    expect(typeof input.message).toBe("string");
    expect(result.toolCalls[0].id).toBe("creative-call");
});

test("原有对象交互保留消息、问题与其他字段", async () => {
    const input = { message: "先确认氛围", questions: [question], brief: [] };
    const result = await recoverCreativeResponse(reply(input), [], context,
        async () => { throw new Error("无需修复"); }, () => {});
    expect(parseToolArguments(result.toolCalls[0].function.arguments)).toEqual(input);
});

test("无效根数组仍经过模型修复，不能直接进入展示", async () => {
    let attempts = 0;
    const result = await recoverCreativeResponse(reply([{ type: "invalid" }]), [], context,
        async () => { attempts++; return reply({ questions: [question] }); }, () => {});
    expect(attempts).toBe(1);
    expect(parseToolArguments(result.toolCalls[0].function.arguments).questions).toEqual([question]);
});
