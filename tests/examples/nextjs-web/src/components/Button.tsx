type ButtonProps = {
  children: React.ReactNode;
  href?: string;
  type?: "button" | "submit";
};

export function Button({ children, href, type = "button" }: ButtonProps) {
  if (href) {
    return <a href={href}>{children}</a>;
  }
  return <button type={type}>{children}</button>;
}
