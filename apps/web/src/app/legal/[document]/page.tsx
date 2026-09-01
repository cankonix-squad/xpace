import type { Metadata } from "next";
import { notFound } from "next/navigation";
import { legalDocuments } from "../legal-content";
import styles from "../legal.module.css";
import { LegalFooter, LegalNav } from "../page";

export function generateStaticParams(){return Object.keys(legalDocuments).map(document=>({document}))}
export async function generateMetadata({params}:{params:Promise<{document:string}>}):Promise<Metadata>{const {document}=await params,item=legalDocuments[document];if(!item)return {};return {title:item.title,description:item.summary,alternates:{canonical:`/legal/${document}`}}}

export default async function LegalDocumentPage({params}:{params:Promise<{document:string}>}){const {document}=await params,item=legalDocuments[document];if(!item)notFound();return <main className={styles.page}><LegalNav/><header className={styles.hero}><p>XSPACE LEGAL</p><h1>{item.title}</h1><span>{item.summary}</span><div className={styles.meta}><span>Version {item.version}</span><span>Last updated 29 August 2026</span><span className={styles.draft}>DRAFT · PENDING APPROVAL</span></div></header><div className={styles.content}><aside className={styles.toc}><strong>On this page</strong>{item.sections.map((section,index)=><a key={section.title} href={`#section-${index+1}`}>{section.title}</a>)}</aside><section className={styles.sections}><div className={styles.review}>This is an operational draft, not a substitute for advice from qualified Indonesian counsel. Commercial publication requires the owner/legal sign-off recorded in the Xspace launch checklist.</div>{item.sections.map((section,index)=><article id={`section-${index+1}`} key={section.title}><h2>{section.title}</h2>{section.paragraphs?.map(paragraph=><p key={paragraph}>{paragraph}</p>)}{section.bullets&&<ul>{section.bullets.map(bullet=><li key={bullet}>{bullet}</li>)}</ul>}</article>)}</section></div><LegalFooter/></main>}
