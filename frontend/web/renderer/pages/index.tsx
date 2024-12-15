import { useRouter } from "next/router";
import React, { useEffect } from "react";

export default function Home() {
	const router = useRouter();

	useEffect(() => {
		router.replace("/chat");
	}, [router]);

	return null; // 或者可以返回一个加载提示
}
