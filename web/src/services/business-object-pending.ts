import { localForageStorageForScope } from "@/lib/localforage-storage";
import type { ObjectWrite } from "./api/business-objects";
const key = "business-object-write-v1";
export async function loadObjectWrite(user: string): Promise<ObjectWrite | null> {
    const raw = await localForageStorageForScope(user).getItem(key);
    if (!raw) return null;
    try {
        if (raw.length > 32768) throw Error();
        const v = JSON.parse(raw) as ObjectWrite;
        if (
            !v ||
            typeof v.clientKey !== "string" ||
            v.clientKey.length < 8 ||
            v.clientKey.length > 128 ||
            !Number.isInteger(v.expectedVersion) ||
            v.expectedVersion < 0 ||
            v.expectedVersion > 100 ||
            (v.objectId !== undefined && (typeof v.objectId !== "string" || !v.objectId.length || v.objectId.length > 80)) ||
            !v.brand ||
            typeof v.brand.name !== "string" ||
            typeof v.brand.audience !== "string" ||
            typeof v.brand.positioning !== "string" ||
            !Array.isArray(v.brand.claims) ||
            !Array.isArray(v.brand.restrictions) ||
            !v.brand.claims.every((item) => typeof item === "string") ||
            !v.brand.restrictions.every((item) => typeof item === "string") ||
            (v.source !== undefined && (!v.source || typeof v.source.objectId !== "string" || v.source.type !== "brand" || v.source.schemaVersion !== 1 || !Number.isInteger(v.source.version) || v.source.version < 1 || v.source.version > 100))
        )
            throw Error();
        return v;
    } catch {
        throw new Error("待确认品牌保存记录损坏，请先核对品牌列表");
    }
}
export const saveObjectWrite = (user: string, request: ObjectWrite) => localForageStorageForScope(user).setItem(key, JSON.stringify(request));
export const clearObjectWrite = (user: string) => localForageStorageForScope(user).removeItem(key);
