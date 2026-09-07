import type { PublicAppearance } from "@/services/api/appearance";

export const DEFAULT_LANDING_POSTER_URL = "/short-drama-styles/future-tech.jpg";

export type LandingMedia = { kind: "video"; src: string; poster: string } | { kind: "image"; src: string };

export function resolveLandingMedia(appearance: Pick<PublicAppearance, "authVideoUrl" | "authVideoPosterUrl">): LandingMedia {
    if (appearance.authVideoUrl) {
        return { kind: "video", src: appearance.authVideoUrl, poster: appearance.authVideoPosterUrl || DEFAULT_LANDING_POSTER_URL };
    }
    return { kind: "image", src: appearance.authVideoPosterUrl || DEFAULT_LANDING_POSTER_URL };
}
