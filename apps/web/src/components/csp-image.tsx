/* eslint-disable @next/next/no-img-element -- Native images are required so production can enforce style-src-attr 'none'. */
import type { ImgHTMLAttributes } from "react";
import type { StaticImageData } from "next/image";

type CspImageProps = Omit<ImgHTMLAttributes<HTMLImageElement>, "src" | "alt"> & {
  src: string | StaticImageData;
  alt: string;
  priority?: boolean;
  fill?: boolean;
  unoptimized?: boolean;
};

export function CspImage({
  src,
  priority = false,
  fill = false,
  unoptimized,
  alt,
  className,
  width,
  height,
  loading,
  ...props
}: CspImageProps) {
  const image = typeof src === "string" ? undefined : src;
  const source = typeof src === "string" ? src : src.src;
  const classes = [className, fill ? "csp-image-fill" : ""]
    .filter(Boolean)
    .join(" ");

  // Native images avoid the inline style attributes emitted by next/image,
  // allowing production to enforce style-src-attr 'none'.
  void unoptimized;
  return (
    <img
      {...props}
      alt={alt}
      className={classes || undefined}
      src={source}
      width={fill ? undefined : (width ?? image?.width)}
      height={fill ? undefined : (height ?? image?.height)}
      loading={priority ? "eager" : loading}
      fetchPriority={priority ? "high" : props.fetchPriority}
    />
  );
}
