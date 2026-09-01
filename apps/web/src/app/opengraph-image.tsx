import { ImageResponse } from "next/og";
import logo from "../asset/Logo-Login-transparent.png";

export const alt = "Xspace — Meet, Collaborate, Communicate, Work";
export const size = { width: 1200, height: 630 };
export const contentType = "image/png";

export default function OpenGraphImage() {
  const logoUrl = new URL(logo.src, "https://xspace.cankonix.com").toString();

  return new ImageResponse(
    <div style={{width:"100%",height:"100%",display:"flex",alignItems:"center",justifyContent:"center",position:"relative",overflow:"hidden",background:"#090d0a",fontFamily:"Helvetica, Arial, sans-serif"}}>
      <div style={{position:"absolute",width:620,height:620,right:-140,top:-220,borderRadius:"50%",background:"radial-gradient(circle, rgba(168,239,69,.24) 0%, rgba(67,111,30,.08) 45%, transparent 70%)"}} />
      <div style={{position:"absolute",inset:34,border:"2px solid #34452f",borderRadius:32,display:"flex"}} />
      <div style={{width:980,display:"flex",flexDirection:"column",alignItems:"center"}}>
        <img src={logoUrl} alt="" width={620} height={214} style={{objectFit:"contain"}} />
        <div style={{marginTop:24,color:"#eff4ed",fontSize:38,fontWeight:600,letterSpacing:"-1px"}}>Secure collaboration, without boundaries.</div>
        <div style={{marginTop:18,color:"#9eaa9f",fontSize:22,letterSpacing:".5px"}}>Meet · Chat · Rooms · Drive · Enterprise Security</div>
        <div style={{marginTop:36,padding:"12px 22px",border:"1px solid #658343",borderRadius:999,color:"#b9ef7b",fontSize:18,letterSpacing:"1.5px"}}>XSPACE.CANKONIX.COM</div>
      </div>
    </div>,
    size,
  );
}
