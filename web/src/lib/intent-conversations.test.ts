import { test, expect } from "vitest";
import { conversationFromHash, conversationLink } from "./intent-conversations";
test("intent conversation permalinks cannot select a different tenant or inject a path", () => {
  const link = conversationLink("commerce", "checkout-chat");
  expect(conversationFromHash(link, "commerce")).toBe("checkout-chat");
  expect(conversationFromHash(link, "other")).toBeNull();
  expect(conversationFromHash(conversationLink("commerce"), "commerce")).toBe(
    "",
  );
  for (const suffix of ["../other", "id?tenant=other", "id/other", "%2e%2e"])
    expect(
      conversationFromHash("#/tenants/commerce/intents/" + suffix, "commerce"),
    ).toBeNull();
});
