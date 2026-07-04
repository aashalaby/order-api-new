import NextAuth from "next-auth";
import Zitadel from "next-auth/providers/zitadel";

// Token custody model (the reason Next.js + BFF was chosen, see HANDOFF):
// the OIDC Authorization Code + PKCE flow terminates HERE, server-side.
// Tokens live only inside the encrypted Auth.js session cookie; they are
// never placed in browser storage and never appear in the session object
// that /api/auth/session serves to the client. BFF route handlers decode
// the cookie server-side (see src/lib/order-api.ts) and attach the Bearer
// token when proxying to the Go API.

// Zitadel puts the project into the access token's `aud` claim when the
// project-audience scope is requested — which is what lets the Go API's
// local JWT validation (ZITADEL_CLIENT_ID mode) accept tokens issued to
// this web app.
const projectAudienceScope = process.env.ZITADEL_PROJECT_ID
  ? ` urn:zitadel:iam:org:project:id:${process.env.ZITADEL_PROJECT_ID}:aud`
  : "";

export const { handlers, auth, signIn, signOut } = NextAuth({
  session: {
    strategy: "jwt",
    // Align with Zitadel's default access-token TTL (12h). There is
    // deliberately no refresh-token rotation yet: when the access token
    // expires the BFF answers 401 and the UI sends the user back through
    // the hosted login (instant if the Zitadel session is still alive).
    maxAge: 12 * 60 * 60,
  },
  providers: [
    Zitadel({
      issuer: process.env.AUTH_ZITADEL_ISSUER,
      // Client id/secret are picked up from AUTH_ZITADEL_ID /
      // AUTH_ZITADEL_SECRET by Auth.js convention — env only, no secrets
      // in code, same rule as the Go service.
      authorization: {
        params: { scope: "openid profile email" + projectAudienceScope },
      },
    }),
  ],
  callbacks: {
    jwt({ token, account }) {
      // `account` is only present on the sign-in round trip; persist the
      // access token into the (encrypted, HttpOnly) session cookie.
      if (account) {
        token.accessToken = account.access_token;
        token.expiresAt = account.expires_at;
      }
      return token;
    },
    // No session callback on purpose: nothing token-shaped may leak into
    // the client-visible session object.
  },
});
