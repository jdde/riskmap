import type { NextAuthOptions } from "next-auth";
import CredentialsProvider from "next-auth/providers/credentials";
import { db } from "./db";

export const authOptions: NextAuthOptions = {
  session: { strategy: "jwt" },
  providers: [
    CredentialsProvider({
      name: "Email",
      credentials: { email: {}, password: {} },
      async authorize(credentials) {
        return db.user.findUnique({ where: { email: credentials?.email } });
      },
    }),
  ],
};
