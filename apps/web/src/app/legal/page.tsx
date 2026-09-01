import type { Metadata } from "next";
import Link from "next/link";
import { legalDocuments } from "./legal-content";
import styles from "./legal.module.css";

export const metadata: Metadata = {title:"Legal Center",description:"Xspace terms, privacy, data processing, cookie, and recording documents.",alternates:{canonical:"/legal"}};

export default function LegalIndex(){return <main className={styles.page}><LegalNav/><section className={styles.index}><p>XSPACE TRUST CENTER</p><h1>Legal & privacy</h1><p>Review how Xspace is provided, how personal data is handled, and the responsibilities that apply to workspace owners, hosts, and participants.</p><div className={styles.review}>DRAFT FOR BUSINESS AND LEGAL APPROVAL — These documents are implemented for review but are not marked final until the contracting entity, registered address, subprocessor list, commercial terms, and owner sign-off are confirmed.</div><div className={styles.grid}>{Object.entries(legalDocuments).map(([slug,document])=><Link className={styles.card} href={`/legal/${slug}`} key={slug}><small>VERSION {document.version}</small><h2>{document.title}</h2><p>{document.summary}</p></Link>)}</div></section><LegalFooter/></main>}

export function LegalNav(){return <header className={styles.nav}><Link className={styles.brand} href="/pricing">Xspace</Link><nav><Link href="/legal/terms">Terms</Link><Link href="/legal/privacy">Privacy</Link><Link href="/legal/dpa">DPA</Link><Link href="/legal/cookies">Cookies</Link><Link href="/legal/recording">Recording</Link></nav></header>}
export function LegalFooter(){return <footer className={styles.footer}><span>© 2026 Cankonix Technology · Draft pending legal approval</span><a href="mailto:info@cankonix.com">info@cankonix.com</a></footer>}
