import type { ReactNode } from "react";
import PageTitle from "../components/ui/PageTitle";
import { FOCUS } from "../components/ui/styles";
import { usePageTitle } from "../hooks/usePageTitle";

const SOURCE = "https://github.com/pscheid92/secretli";

function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="space-y-3">
      <h2 className="m-0 text-lead font-semibold tracking-[-0.01em] text-ink">{title}</h2>
      <div className="space-y-3 text-pretty text-body text-muted">{children}</div>
    </section>
  );
}

function Code({ children }: { children: ReactNode }) {
  return <code className="font-mono text-sm text-ink">{children}</code>;
}

function External({ href, children }: { href: string; children: ReactNode }) {
  return (
    <a
      href={href}
      target="_blank"
      rel="noreferrer"
      className={`rounded-sm text-ink underline decoration-accent decoration-2 underline-offset-4 ${FOCUS}`}
    >
      {children}
    </a>
  );
}

/**
 * What happens to a secret, in the order it happens. Every number and name
 * here is the one in the code: change the code, change this page.
 */
export default function HowItWorksPage() {
  usePageTitle("How it works");
  return (
    <div className="space-y-10">
      <PageTitle lead="Secretli never sees your secret. This is what happens instead, in order.">
        How it works
      </PageTitle>

      <Section title="The key never leaves your browser">
        <p>
          When you create a link, your browser draws a random 256-bit secret. From it, HKDF-SHA512
          derives everything else: the key that encrypts your text or files, the key for the
          metadata, the public ID the server files everything under, and the tokens that prove you
          may read or delete it.
        </p>
        <p>
          The secret itself goes only into the part of the link after <Code>#</Code>. Browsers never
          send that part to any server, not to this one and not to the one unfurling a link preview
          in your chat app.
        </p>
      </Section>

      <Section title="What the server stores">
        <p>
          Ciphertext, encrypted in your browser with XChaCha20-Poly1305, in records of 4 MiB that
          are bound to their position so nothing can be reordered or dropped. The file names and
          sizes sit inside an encrypted manifest. Next to it: the total size, the expiry, whether
          the link opens once, and hashes of the tokens.
        </p>
        <p>
          Once a secret is gone, a note of what became of it stays for a week: opened, expired or
          deleted, and when. It holds no content, and only a link to the secret can read it. That is
          how the owner link, and the link itself, can tell you what happened.
        </p>
        <p>
          That is all. The server cannot read the content, cannot see the file names, and cannot
          tell a note from a file.
        </p>
      </Section>

      <Section title="Opens once">
        <p>
          Links open once unless you choose otherwise. The moment the recipient opens one, the
          server marks it used and refuses every request after that. For files, the download window
          stays open for 15 minutes so the transfer can finish; then the data is gone.
        </p>
        <p>
          Everything expires, from 5 minutes to 7 days after you made it. A cleanup runs every
          minute and removes what has run out.
        </p>
      </Section>

      <Section title="Passwords">
        <p>
          With a password, the key for the content is derived from the secret and the password
          together, using scrypt, in your browser. The password never leaves it either. The server
          cannot check a password: a wrong one simply fails to unlock, and trying does not use up a
          one-time secret.
        </p>
      </Section>

      <Section title="The owner link">
        <p>
          Your owner link is the recipient's link with a deletion token after <Code>!</Code>. It
          opens the secret too, so keep it to yourself. What it adds is the right to delete the
          secret for everyone, and word of what became of it: opened, and when; expired unopened; or
          deleted by you.
        </p>
      </Section>

      <Section title="QR codes and codes">
        <p>
          The QR code is the link as an image; the other device reads it with its camera, in its own
          browser.
        </p>
        <p>
          The short code, a number and two words, hands a link to another device without typing it.
          The two browsers run a password-authenticated key exchange (CPace on ristretto255) with
          the two words as the password, prove to each other that they typed the same ones, and only
          then pass the link through the server, encrypted. The server sees neither the words nor
          the link. A code is good for 10 minutes, and a wrong one gives nothing away.
        </p>
      </Section>

      <Section title="What you can check yourself">
        <p>
          The <External href={SOURCE}>source code</External> is public, including the page you are
          reading. The server images are built in the open, carry an SBOM and SLSA provenance, and
          are signed with cosign. This page allows no scripts, styles or fonts from anywhere but
          this site, and sends no referrer when you follow a link out.
        </p>
      </Section>
    </div>
  );
}
