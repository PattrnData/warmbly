import { beforeEach, describe, expect, it, vi } from "vitest";
import onboardOAuthStart from "./onboardOAuthStart";
import Request from "../../Request";

vi.mock("../../Request", () => ({ default: vi.fn() }));

const request = vi.mocked(Request);

describe("Outlook delegated reconnect start", () => {
    beforeEach(() => {
        request.mockReset();
        request.mockResolvedValue({ url: "https://example.test/consent", state: "fixture-state" });
    });

    it("uses the reconnect endpoint with the account_id required by the handler", async () => {
        await onboardOAuthStart("outlook", "fixture-account-id");
        expect(request).toHaveBeenCalledWith(expect.objectContaining({
            method: "POST",
            url: "/emails/onboarding/oauth/reconnect/start",
            data: { account_id: "fixture-account-id" },
            authorization: true,
        }));
    });

    it("preserves the normal connect route and provider payload", async () => {
        await onboardOAuthStart("gmail");
        expect(request).toHaveBeenCalledWith(expect.objectContaining({
            url: "/emails/onboarding/oauth/start",
            data: { provider: "gmail" },
        }));
    });
});
