import Request from "../../Request";

export interface OAuthStartResponse {
    url: string;
    state: string;
}

export default async function onboardOAuthStart(provider: "gmail" | "outlook", reconnectEmailID?: string): Promise<OAuthStartResponse> {
    return await Request<OAuthStartResponse>({
        method: "POST",
        url: reconnectEmailID ? `/emails/onboarding/oauth/reconnect/start` : `/emails/onboarding/oauth/start`,
        data: reconnectEmailID ? { account_id: reconnectEmailID } : { provider },
        authorization: true,
    });
}
