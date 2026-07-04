import "next-auth/jwt";

declare module "next-auth/jwt" {
  interface JWT {
    /** Zitadel access token (JWT), forwarded by the BFF to the Go API. */
    accessToken?: string;
    /** Unix seconds when accessToken expires. */
    expiresAt?: number;
  }
}
