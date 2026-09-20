import { compactApiParams, http } from "./request";

export type ObjectReference = { objectId: string; version: number; type: "brand"; schemaVersion: 1 };
export type Brand = { name: string; audience: string; positioning: string; claims: string[]; restrictions: string[] };
export type BusinessObject = { objectId: string; type: "brand"; title: string; version: number; archived: boolean };
export type ObjectView = { reference: ObjectReference; brand: Brand; digest: string; source?: ObjectReference; archived: boolean; currentVersion: number };
export type ObjectWrite = { objectId?: string; expectedVersion: number; clientKey: string; brand: Brand; source?: ObjectReference };
export const listBusinessObjects = (q = "", archived = false, offset = 0, signal?: AbortSignal) => http.get<BusinessObject[]>("/business-objects", { params: compactApiParams({ q, archived: String(archived), offset }), signal });
export const readBusinessObject = (ref: ObjectReference, signal?: AbortSignal) => http.get<ObjectView>(`/business-objects/${encodeURIComponent(ref.objectId)}/versions/${ref.version}`, { signal });
export const saveBusinessObject = (input: ObjectWrite) => http.post<ObjectView>("/business-objects", input);
export const archiveBusinessObject = (objectId: string, expectedVersion: number, archived: boolean) => http.patch(`/business-objects/${encodeURIComponent(objectId)}/archive`, { expectedVersion, archived });
