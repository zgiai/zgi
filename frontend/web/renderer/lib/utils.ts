import { type ClassValue, clsx } from "clsx";
import { twMerge } from "tailwind-merge";

export function cn(...inputs: ClassValue[]) {
	return twMerge(clsx(inputs));
}

/**
 * Check if current environment is desktop
 */
export const isDesktop = () => {
	if (typeof window === "undefined") return false;
	return typeof window.ipc !== "undefined";
};
