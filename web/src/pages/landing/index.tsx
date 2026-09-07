import { useEffect, useMemo, useRef, useState, type FormEvent } from "react";
import { ArrowRight, ArrowUpRight, Check, ChevronDown, CirclePlay, Image as ImageIcon, LoaderCircle, MoreHorizontal, Moon, Sun, Pause, Play, Sparkles } from "lucide-react";
import { Link, useNavigate } from "react-router";

import { BrandLogoFrame } from "@/components/brand/brand-logo";
import { SiteComplianceFooter } from "@/components/layout/site-compliance-footer";
import { brandStudioLabel, useAppearanceStore } from "@/stores/use-appearance-store";
import { useThemeStore } from "@/stores/use-theme-store";
import { saveLandingDraft } from "@/lib/landing-draft";
import { DEFAULT_LANDING_POSTER_URL, resolveLandingMedia } from "./landing-media";
import { useUserStore } from "@/stores/use-user-store";
import "./landing.css";

const creativeSteps = [
    { number: "01", label: "文字", title: "写下一个想法" },
    { number: "02", label: "角色", title: "建立人物与世界" },
    { number: "03", label: "分镜", title: "组织每一帧画面" },
    { number: "04", label: "成片", title: "让故事继续发生" },
];

type AgentShowcaseStageKind = "orchestration" | "input" | "decisions" | "review";

type AgentShowcaseStage = {
    number: string;
    eyebrow: string;
    title: string;
    description: string;
    kind: AgentShowcaseStageKind;
    panelTitle: string;
    panelFooter: string;
    image: string;
};

/** 章节文案和占位素材集中在这里，替换官网素材时不需要改面板组件。 */
const agentShowcaseStages: AgentShowcaseStage[] = [
    {
        number: "01",
        eyebrow: "LONG VIDEO AGENT",
        title: "统筹长视频生成",
        description: "从一句创作输入开始，Agent 自动完成故事、脚本、镜头、素材和生成任务，同时保留实时交互修改能力。",
        kind: "orchestration",
        panelTitle: "Agent 创作会话",
        panelFooter: "自动化制作",
        image: "/short-drama-styles/urban-live-action.jpg",
    },
    {
        number: "02",
        eyebrow: "ADAPTIVE INPUT",
        title: "适应不同创作输入",
        description: "创作输入可能只有一句话，也可能已经包含完整故事和制作要求。启幕会扩充简单输入，保留详细输入中的已有决定，只补足缺失部分，再进入同一套规划和执行流程。",
        kind: "input",
        panelTitle: "创作输入",
        panelFooter: "制作计划",
        image: "/short-drama-styles/storybook-fantasy.jpg",
    },
    {
        number: "03",
        eyebrow: "STRUCTURED DECISIONS",
        title: "组织创作决策",
        description: "启幕基于领域知识，把故事、剧本和制作方案中的关键决定写成结构化产物，使前一阶段的创作决定能被后续阶段准确读取和执行。",
        kind: "decisions",
        panelTitle: "结构化产物",
        panelFooter: "可评审可执行",
        image: "/short-drama-styles/future-tech.jpg",
    },
    {
        number: "04",
        eyebrow: "LAYERED REVIEW",
        title: "评审文字与媒体",
        description: "启幕同时检查文字规划和媒体结果：先核对故事、剧本和制作方案，再借助视觉理解检查参考图、片段和全片的角色与场景连贯性。",
        kind: "review",
        panelTitle: "评审",
        panelFooter: "文字与视觉共同检查",
        image: "/short-drama-styles/suspense-noir.jpg",
    },
];

const agentShowcaseMedia = {
    reference: "/short-drama-styles/suspense-noir.jpg",
    shot: "/short-drama-styles/urban-live-action.jpg",
    full: "/short-drama-styles/future-tech.jpg",
};

const fallbackWorks = [
    { title: "城市入夜之前", meta: "现实电影感", image: "/short-drama-styles/urban-live-action.jpg" },
    { title: "一场发生在月面的演出", meta: "科幻叙事", image: "/short-drama-styles/space-opera.jpg" },
    { title: "梦里有一座旧剧院", meta: "故事书幻想", image: "/short-drama-styles/storybook-fantasy.jpg" },
    { title: "霓虹雨中的追逐", meta: "赛博朋克", image: "/short-drama-styles/cyberpunk-neon.jpg" },
    { title: "留给下一帧的光", meta: "未来影像", image: "/short-drama-styles/future-tech.jpg" },
    { title: "风从东方来", meta: "东方叙事", image: "/short-drama-styles/chinese-2d.jpg" },
];

export default function LandingPage() {
    const navigate = useNavigate();
    const appearance = useAppearanceStore((state) => state.appearance);
    const { brandName } = appearance;
    const theme = useThemeStore((state) => state.theme);
    const setTheme = useThemeStore((state) => state.setTheme);
    const user = useUserStore((state) => state.user);
    const heroVideoRef = useRef<HTMLVideoElement>(null);
    const heroPromptInputRef = useRef<HTMLInputElement>(null);
    const agentStageRefs = useRef<Array<HTMLElement | null>>([]);
    const landingMedia = useMemo(() => resolveLandingMedia(appearance), [appearance.authVideoUrl, appearance.authVideoPosterUrl]);
    const [reducedMotion, setReducedMotion] = useState(false);
    const [heroMediaFailed, setHeroMediaFailed] = useState(false);
    const [failedPoster, setFailedPoster] = useState("");
    const [showBackgroundImage, setShowBackgroundImage] = useState(false);
    const [heroVideoPlaying, setHeroVideoPlaying] = useState(false);
    const [activeAgentStage, setActiveAgentStage] = useState(0);
    const [heroPrompt, setHeroPrompt] = useState("");
    const [heroPromptSubmitting, setHeroPromptSubmitting] = useState(false);
    const [heroPromptError, setHeroPromptError] = useState("");


    const primaryHref = user ? "/home" : "/login?next=%2Fhome";
    const createHref = user ? "/home" : "/login?next=%2Fhome";
    const tvLabel = "影像灵感";

    useEffect(() => {
        if (landingMedia.kind !== "video") return;
        const poster = new Image();
        poster.onerror = () => setFailedPoster(landingMedia.poster);
        poster.src = landingMedia.poster;
        return () => { poster.onerror = null; };
    }, [landingMedia]);

    useEffect(() => {
        setHeroMediaFailed(false);
        setHeroVideoPlaying(false);
    }, [landingMedia.src]);

    useEffect(() => {
        const query = window.matchMedia("(prefers-reduced-motion: reduce)");
        const update = () => setReducedMotion(query.matches);
        update();
        query.addEventListener("change", update);
        return () => query.removeEventListener("change", update);
    }, []);

    useEffect(() => {
        if (landingMedia.kind !== "video" || heroMediaFailed || showBackgroundImage) return;
        const video = heroVideoRef.current;
        if (!video) return;
        if (reducedMotion) {
            video.pause();
            setHeroVideoPlaying(false);
            return;
        }
        void video.play().catch(() => setHeroVideoPlaying(false));
    }, [heroMediaFailed, landingMedia.kind, landingMedia.src, reducedMotion, showBackgroundImage]);

    useEffect(() => {
        const elements = agentStageRefs.current.filter((element): element is HTMLElement => Boolean(element));
        if (!elements.length || typeof IntersectionObserver === "undefined") return;
        const observer = new IntersectionObserver(
            (entries) => {
                const visible = entries.filter((entry) => entry.isIntersecting).sort((left, right) => right.intersectionRatio - left.intersectionRatio)[0];
                if (visible) setActiveAgentStage(Number((visible.target as HTMLElement).dataset.index || 0));
            },
            { rootMargin: "-34% 0px -46% 0px", threshold: [0.15, 0.35, 0.6] },
        );
        elements.forEach((element) => observer.observe(element));
        return () => observer.disconnect();
    }, []);

    const toggleHeroVideo = () => {
        if (heroMediaFailed || showBackgroundImage) {
            setHeroMediaFailed(false);
            setShowBackgroundImage(false);
            return;
        }
        const video = heroVideoRef.current;
        if (!video) return;
        if (video.paused) {
            void video
                .play()
                .then(() => setHeroVideoPlaying(true))
                .catch(() => setHeroVideoPlaying(false));
        } else {
            video.pause();
            setHeroVideoPlaying(false);
        }
    };

    const handleHeroPromptSubmit = async (event: FormEvent<HTMLFormElement>) => {
        event.preventDefault();
        const prompt = heroPrompt.trim();
        if (!prompt) {
            setHeroPromptError("请先描述你的镜头、角色或故事");
            heroPromptInputRef.current?.focus();
            return;
        }
        if (heroPromptSubmitting) return;
        setHeroPromptSubmitting(true);
        setHeroPromptError("");
        try {
            saveLandingDraft(prompt);
            navigate(primaryHref);
        } catch (error) {
            setHeroPromptError(error instanceof Error && error.message ? error.message : "暂时无法保存创作内容，请重试");
        } finally {
            setHeroPromptSubmitting(false);
        }
    };

    const selectAgentStage = (index: number) => {
        setActiveAgentStage(index);
        const stage = agentStageRefs.current[index];
        stage?.scrollIntoView({ behavior: reducedMotion ? "auto" : "smooth", block: "center" });
    };

    return (
        <main className="landing-page">
            <a href="#landing-content" className="landing-skip-link">
                跳转到主要内容
            </a>

            <section className="landing-hero" aria-labelledby="landing-title">
                <div className="landing-hero-media" aria-hidden="true">
                    {landingMedia.kind === "video" && !heroMediaFailed && !showBackgroundImage ? (
                        <video
                            key={landingMedia.src}
                            ref={heroVideoRef}
                            src={landingMedia.src}
                            poster={failedPoster === landingMedia.poster ? DEFAULT_LANDING_POSTER_URL : landingMedia.poster}
                            autoPlay={!reducedMotion}
                            muted
                            loop
                            playsInline
                            preload="metadata"
                            onPlay={() => setHeroVideoPlaying(true)}
                            onPause={() => setHeroVideoPlaying(false)}
                            onError={() => {
                                setHeroMediaFailed(true);
                                setHeroVideoPlaying(false);
                            }}
                        />
                    ) : (
                        <LandingImage src={landingMedia.kind === "image" ? landingMedia.src : landingMedia.poster} fallbackSrc={DEFAULT_LANDING_POSTER_URL} alt="" className="landing-hero-media-fallback" loading="eager" />
                    )}
                </div>
                <div className="landing-hero-scrim" aria-hidden="true" />

                <header className="landing-header landing-container">
                    <Link to="/" className="landing-brand" aria-label={`${brandName}首页`}>
                        <BrandLogoFrame className="landing-brand-mark" logoClassName="landing-logo-image" fallback={<span className="landing-logo-fallback" />} />
                        <span className="landing-brand-copy">
                            <strong>{brandName}</strong>
                            <small>{brandStudioLabel(appearance)}</small>
                        </span>
                    </Link>
                    <nav className="landing-nav" aria-label="官网导航">
                        <a href="#landing-capabilities">创作能力</a>
                        <a href="#landing-works">{tvLabel}</a>
                        <a href="#landing-about">关于{brandName}</a>
                    </nav>
                    <div className="landing-header-actions">
                        <button type="button" className="landing-theme-toggle" onClick={() => setTheme(theme === "dark" ? "light" : "dark")} aria-label={theme === "dark" ? "切换到浅色主题" : "切换到深色主题"} title={theme === "dark" ? "切换到浅色主题" : "切换到深色主题"}>{theme === "dark" ? <Sun /> : <Moon />}</button>
                        <Link to={createHref} className="landing-header-login">
                            {user ? "进入工作台" : "登录"}
                        </Link>
                        <Link to={primaryHref} className="landing-header-cta">
                            开始创作 <ArrowUpRight aria-hidden="true" />
                        </Link>
                    </div>
                </header>

                <div className="landing-hero-content landing-container">
                    <div className="landing-hero-copy">
                        <p className="landing-kicker">
                            <Sparkles aria-hidden="true" /> {brandStudioLabel(appearance)}
                        </p>
                        <h1 id="landing-title">{brandName}</h1>
                        <p className="landing-hero-tagline">{appearance.authHeroTitle}</p>
                        <p className="landing-hero-description">{appearance.authHeroDescription || "把剧本、角色、分镜与生成任务，放进同一个 AI 影视创作片场。"}</p>
                        <div className="landing-hero-prompt-wrap">
                            <form className="landing-hero-prompt" onSubmit={handleHeroPromptSubmit} aria-busy={heroPromptSubmitting}>
                                <span className="landing-hero-prompt-copy">
                                    <label htmlFor="landing-hero-prompt-input">从一个画面开始</label>
                                    <input
                                        ref={heroPromptInputRef}
                                        id="landing-hero-prompt-input"
                                        className="landing-hero-prompt-input"
                                        type="text"
                                        value={heroPrompt}
                                        onChange={(event) => {
                                            setHeroPrompt(event.target.value);
                                            if (heroPromptError) setHeroPromptError("");
                                        }}
                                        placeholder="描述你的镜头、角色或故事"
                                        autoComplete="off"
                                        maxLength={2000}
                                        aria-describedby={heroPromptError ? "landing-hero-prompt-error" : undefined}
                                    />
                                </span>
                                <button type="submit" className="landing-hero-prompt-action" disabled={heroPromptSubmitting}>
                                    {heroPromptSubmitting ? "准备中" : "开始创作"} <ArrowRight aria-hidden="true" />
                                </button>
                            </form>
                            {heroPromptError ? (
                                <p id="landing-hero-prompt-error" className="landing-hero-prompt-error" role="alert">
                                    {heroPromptError}
                                </p>
                            ) : null}
                        </div>
                        <div className="landing-hero-secondary-actions">
                            <a href="#landing-story" className="landing-hero-explore">
                                <CirclePlay aria-hidden="true" /> 看见创作如何发生
                            </a>
                            {landingMedia.kind === "video" ? (
                                <>
                                <button type="button" className="landing-hero-video-toggle" onClick={toggleHeroVideo} aria-label={heroVideoPlaying ? "暂停首页背景视频" : "播放首页背景视频"} aria-pressed={heroVideoPlaying}>
                                    {heroVideoPlaying ? <Pause aria-hidden="true" /> : <Play aria-hidden="true" />}
                                    {heroVideoPlaying ? "暂停影片" : "播放影片"}
                                </button>
                                <button type="button" className="landing-hero-video-toggle" title="切换背景图片" aria-label="切换背景图片" aria-pressed={showBackgroundImage} onClick={() => { setShowBackgroundImage((value) => !value); setHeroVideoPlaying(false); }}>
                                    <ImageIcon aria-hidden="true" />
                                </button>
                                </>
                            ) : null}
                        </div>
                    </div>
                    <div className="landing-hero-index" aria-label="首页场景进度">
                        <span className="is-active">01</span>
                        <i aria-hidden="true" />
                        <span>04</span>
                    </div>
                </div>

                <a href="#landing-content" className="landing-scroll-cue" aria-label="向下浏览首页内容">
                    <span>向下探索</span>
                    <ChevronDown aria-hidden="true" />
                </a>
            </section>

            <div id="landing-content">
                <section id="landing-story" className="landing-story" aria-labelledby="landing-story-title">
                    <div className="landing-container">
                        <div className="landing-section-kicker">
                            <span>01</span>
                            <span>FROM IDEA TO FRAME</span>
                        </div>
                        <div className="landing-story-heading">
                            <h2 id="landing-story-title">
                                创作从一个想法开始，
                                <br />
                                <em>而不是从工具开始。</em>
                            </h2>
                            <p>{brandName}把影视创作拆成一条可以继续的链路，让每一次尝试都留下下一帧的入口。</p>
                        </div>
                        <div className="landing-process-grid">
                            {creativeSteps.map((step, index) => (
                                <div className="landing-process-step" key={step.number}>
                                    <div className="landing-process-step-top">
                                        <span>{step.number}</span>
                                        {index < creativeSteps.length - 1 ? <ArrowRight aria-hidden="true" /> : null}
                                    </div>
                                    <small>{step.label}</small>
                                    <strong>{step.title}</strong>
                                </div>
                            ))}
                        </div>
                    </div>
                </section>

                <section id="landing-capabilities" className="landing-capabilities landing-agent-showcase" aria-labelledby="landing-capabilities-title">
                    <div className="landing-container">
                        <div className="landing-section-kicker">
                            <span>02</span>
                            <span>THE AGENT WORKFLOW</span>
                        </div>
                        <div className="landing-agent-heading">
                            <h2 id="landing-capabilities-title">
                                把每一帧，
                                <br />
                                <em>变成下一步的起点。</em>
                            </h2>
                            <p>{brandName}把输入、决策、评审和成片组织成一条可以继续的创作链。</p>
                        </div>
                        <div className="landing-agent-step-nav" role="group" aria-label="Agent 创作流程章节">
                            {agentShowcaseStages.map((stage, index) => (
                                <button
                                    key={stage.number}
                                    type="button"
                                    id={`landing-agent-stage-tab-${stage.number}`}
                                    aria-current={activeAgentStage === index ? "step" : undefined}
                                    aria-controls={`landing-agent-stage-${stage.number}`}
                                    className={activeAgentStage === index ? "is-active" : ""}
                                    onClick={() => selectAgentStage(index)}
                                >
                                    <span>{stage.number}</span>
                                    <strong>{stage.title}</strong>
                                </button>
                            ))}
                        </div>
                        <div className="landing-agent-stage-list">
                            {agentShowcaseStages.map((stage, index) => (
                                <article
                                    ref={(element) => {
                                        agentStageRefs.current[index] = element;
                                    }}
                                    className={`landing-agent-stage${activeAgentStage === index ? " is-active" : ""}`}
                                    data-index={index}
                                    id={`landing-agent-stage-${stage.number}`}
                                    key={stage.number}
                                >
                                    <div className="landing-agent-stage-copy">
                                        <span className="landing-agent-stage-number">{stage.number}</span>
                                        <p className="landing-agent-stage-eyebrow">{stage.eyebrow}</p>
                                        <h3>{stage.title}</h3>
                                        <p>{stage.description.replaceAll("启幕", brandName)}</p>
                                        <div className="landing-agent-progress" aria-label={`当前展示第 ${index + 1} 个章节，共 ${agentShowcaseStages.length} 个章节`}>
                                            <span>0{index + 1}</span>
                                            <div>
                                                <i style={{ transform: `scaleX(${(index + 1) / agentShowcaseStages.length})` }} />
                                            </div>
                                            <span>0{agentShowcaseStages.length}</span>
                                        </div>
                                        <Link to={primaryHref} className="landing-inline-link">
                                            进入创作 <ArrowUpRight aria-hidden="true" />
                                        </Link>
                                    </div>
                                    <AgentShowcasePanel stage={stage} />
                                </article>
                            ))}
                        </div>
                    </div>
                </section>

                <section id="landing-works" className="landing-works" aria-labelledby="landing-works-title">
                    <div className="landing-container">
                        <div className="landing-works-heading">
                            <div>
                                <div className="landing-section-kicker">
                                    <span>03</span>
                                    <span>THE SCREENING ROOM</span>
                                </div>
                                <h2 id="landing-works-title">
                                    {tvLabel} <em>·</em> 探索不同风格
                                </h2>
                            </div>
                            <Link to={primaryHref} className="landing-inline-link">
                                开始创作 <ArrowRight aria-hidden="true" />
                            </Link>
                        </div>
                        <p className="landing-works-description">从不同的影像风格出发，寻找属于你的故事。</p>
                        <div className="landing-work-grid" aria-live="polite">
                            {fallbackWorks.map((work, index) => <LandingWorkCard key={work.title} fallback={work} index={index} href={primaryHref} />)}
                        </div>
                    </div>
                </section>

                <section id="landing-about" className="landing-final-cta" aria-labelledby="landing-final-title">
                    <LandingImage src="/short-drama-styles/future-tech.jpg" fallbackSrc="/short-drama-styles/space-opera.jpg" alt="" className="landing-final-cta-image" loading="lazy" />
                    <div className="landing-final-cta-scrim" aria-hidden="true" />
                    <div className="landing-container landing-final-cta-content">
                        <p className="landing-kicker">
                            <Sparkles aria-hidden="true" /> {brandStudioLabel(appearance)}
                        </p>
                        <h2 id="landing-final-title">
                            现在，开始你的
                            <br />
                            <span>第一个镜头。</span>
                        </h2>
                        <p>你的下一个故事，值得有一片可以继续生长的空间。</p>
                        <Link to={primaryHref} className="landing-final-cta-button">
                            立即进入创作 <ArrowUpRight aria-hidden="true" />
                        </Link>
                    </div>
                </section>
            </div>

            <footer className="landing-footer">
                <div className="landing-container landing-footer-inner">
                    <Link to="/" className="landing-brand landing-footer-brand" aria-label={`${brandName}首页`}>
                        <BrandLogoFrame className="landing-brand-mark" logoClassName="landing-logo-image" fallback={<span className="landing-logo-fallback" />} />
                        <span className="landing-brand-copy">
                            <strong>{brandName}</strong>
                            <small>{brandStudioLabel(appearance)}</small>
                        </span>
                    </Link>
                    <nav className="landing-footer-links" aria-label="页脚导航">
                        <Link to={createHref}>{user ? "工作台" : "登录"}</Link>
                        <Link to={primaryHref}>开始创作</Link>
                        <Link to={primaryHref}>{tvLabel}</Link>
                        <a href="#landing-about">关于{brandName}</a>
                    </nav>
                    <SiteComplianceFooter className="landing-footer-copy" />
                </div>
            </footer>
        </main>
    );
}

function AgentShowcasePanel({ stage }: { stage: AgentShowcaseStage }) {
    return (
        <div className={`landing-agent-panel landing-agent-panel-${stage.kind}`}>
            <div className="landing-agent-window-bar">
                <span className="landing-agent-window-dots" aria-hidden="true">
                    <i />
                    <i />
                    <i />
                </span>
                <strong>{stage.panelTitle}</strong>
                <span className="landing-agent-window-status" aria-hidden="true" />
            </div>
            <div className="landing-agent-panel-content">
                {stage.kind === "orchestration" ? <AgentOrchestrationPanel /> : null}
                {stage.kind === "input" ? <AgentInputPanel /> : null}
                {stage.kind === "decisions" ? <AgentDecisionsPanel /> : null}
                {stage.kind === "review" ? <AgentReviewPanel stage={stage} /> : null}
            </div>
            <div className="landing-agent-panel-footer">
                <span className="landing-agent-footer-rule" aria-hidden="true">
                    <i />
                </span>
                <strong>{stage.panelFooter}</strong>
            </div>
        </div>
    );
}

function AgentOrchestrationPanel() {
    const steps = [
        ["读取创作输入", "理解创作要求、时长与画面方向。", "16:9 · 4:02"],
        ["发展故事与剧本", "形成完整的故事和剧本。", "15 场"],
        ["准备生成任务", "为角色、场景和片段建立生成任务与必要关联。", "16 资产 · 25 片段"],
        ["检查并执行", "确认制作方案完整后交给生成模型执行。", "Lite · 生成中"],
    ];
    return (
        <div className="landing-agent-orchestration">
            <div className="landing-agent-panel-topline">
                <span>Agent</span>
                <span>PRO MODE</span>
            </div>
            <div className="landing-agent-orchestration-brief">
                <div className="landing-agent-brief-heading">
                    <span>创作输入</span>
                    <MoreHorizontal aria-hidden="true" />
                </div>
                <p>我想做一支约四分钟的写实街头犯罪片。黎明前，一名青年在潮湿破败的工业城区遭到追杀，枪战沿公寓、便利店和高架铁路展开。让爵士乐贯穿全片，并保留低饱和胶片质感。</p>
            </div>
            <div className="landing-agent-orchestration-list">
                {steps.map(([title, description, meta], index) => (
                    <div className="landing-agent-orchestration-step" key={title}>
                        <span className={`landing-agent-step-state${index === steps.length - 1 ? " is-running" : ""}`} aria-hidden="true">
                            {index === steps.length - 1 ? <LoaderCircle /> : <Check />}
                        </span>
                        <div>
                            <strong>{title}</strong>
                            <p>{description}</p>
                        </div>
                        <small>{meta}</small>
                    </div>
                ))}
            </div>
        </div>
    );
}

function AgentInputPanel() {
    const decisions = [
        ["故事", "争吵后的恋人因停电被困在便利店，从沉默走向和解。"],
        ["节拍", "街区停电 → 被迫交谈 → 天台坦白 → 来电后并肩离开"],
        ["画面", "二维动画，蓝紫雨夜，角色造型固定，结尾转为暖光。"],
        ["声音", "雨声与电流声贯穿，坦白前留白，来电后进入轻爵士。"],
        ["镜头", "停电远景，交替近景，天台缓慢推进，街道全景收束。"],
    ];
    return (
        <div className="landing-agent-input-grid">
            <article className="landing-agent-panel-card landing-agent-idea-card">
                <h3>一句想法</h3>
                <p>城市停电时，两个人重新爱上彼此。</p>
                <div className="landing-agent-card-action">
                    <span>Agent</span>
                    <strong>补全缺失决策</strong>
                </div>
            </article>
            <article className="landing-agent-panel-card landing-agent-detail-card">
                <h3>详细输入</h3>
                <p className="landing-agent-detail-title">《停电之后》 · 3:00 · 4 个场景 · 24 个镜头</p>
                <dl>
                    {decisions.map(([term, definition]) => (
                        <div key={term}>
                            <dt>{term}</dt>
                            <dd>{definition}</dd>
                        </div>
                    ))}
                </dl>
                <div className="landing-agent-card-action">
                    <span>Agent</span>
                    <strong>承接已有决策</strong>
                </div>
            </article>
        </div>
    );
}

function AgentDecisionsPanel() {
    const knowledge = ["故事", "镜头", "节奏", "声音"];
    const plan = ["结构", "场景", "节奏", "声音"];
    return (
        <div className="landing-agent-decision-grid">
            <div className="landing-agent-knowledge-column">
                <strong>领域知识</strong>
                {knowledge.map((item, index) => (
                    <div className={`landing-agent-knowledge-item${index === 0 ? " is-selected" : ""}`} key={item}>
                        <span>{item}</span>
                        <Check aria-hidden="true" />
                    </div>
                ))}
            </div>
            <div className="landing-agent-plan-column">
                <div className="landing-agent-plan-heading">
                    <strong>制作方案</strong>
                    <Check aria-hidden="true" />
                </div>
                {plan.map((item) => (
                    <div className="landing-agent-plan-row" key={item}>
                        <span>{item}</span>
                        <i aria-hidden="true" />
                    </div>
                ))}
            </div>
        </div>
    );
}

function AgentReviewPanel({ stage }: { stage: AgentShowcaseStage }) {
    const checks = [
        ["故事", "通过"],
        ["剧本", "检查中"],
        ["制作方案", "通过"],
    ];
    return (
        <div className="landing-agent-review">
            <div className="landing-agent-review-tabs" role="group" aria-label="评审类型">
                <span className="is-active">文字检查</span>
                <span>视觉检查</span>
            </div>
            <div className="landing-agent-review-grid">
                <div className="landing-agent-review-checks">
                    <div className="landing-agent-review-heading">
                        <strong>文字检查</strong>
                        <span className="landing-agent-live-dot" aria-hidden="true" />
                    </div>
                    {checks.map(([label, status], index) => (
                        <div className={`landing-agent-review-check${index === 1 ? " is-checking" : ""}`} key={label}>
                            <span>{label}</span>
                            {index === 1 ? <LoaderCircle aria-hidden="true" /> : <Check aria-hidden="true" />}
                            <small>{status}</small>
                        </div>
                    ))}
                    <div className="landing-agent-review-line" aria-hidden="true" />
                </div>
                <div className="landing-agent-review-media">
                    <figure>
                        <LandingImage src={stage.image} fallbackSrc={agentShowcaseMedia.full} alt="视觉评审参考图" className="landing-agent-image" loading="lazy" />
                        <figcaption>参考图</figcaption>
                    </figure>
                    <figure>
                        <LandingImage src={agentShowcaseMedia.shot} fallbackSrc={agentShowcaseMedia.full} alt="视觉评审片段" className="landing-agent-image" loading="lazy" />
                        <figcaption>片段</figcaption>
                    </figure>
                    <figure>
                        <LandingImage src={agentShowcaseMedia.full} fallbackSrc={agentShowcaseMedia.reference} alt="视觉评审全片" className="landing-agent-image" loading="lazy" />
                        <figcaption>全片</figcaption>
                    </figure>
                </div>
            </div>
        </div>
    );
}

function LandingWorkCard({ fallback, index, href }: { fallback: (typeof fallbackWorks)[number]; index: number; href: string }) {
    return (
        <Link to={href} className={`landing-work-card landing-work-card-${index % 3}`}>
            <div className="landing-work-media">
                <LandingImage src={fallback.image} fallbackSrc="/short-drama-styles/future-tech.jpg" alt={fallback.title} loading="lazy" />
                <span className="landing-work-play" aria-hidden="true"><ArrowUpRight /></span>
                <span className="landing-work-index">0{index + 1}</span>
            </div>
            <div className="landing-work-meta"><strong>{fallback.title}</strong><span>{fallback.meta}</span></div>
        </Link>
    );
}

function LandingImage({ src, fallbackSrc, alt, className = "", loading = "lazy" }: { src: string; fallbackSrc: string; alt: string; className?: string; loading?: "lazy" | "eager" }) {
    const [currentSrc, setCurrentSrc] = useState(src);
    useEffect(() => setCurrentSrc(src), [src]);
    return (
        <img
            className={className}
            src={currentSrc}
            alt={alt}
            loading={loading}
            onError={() => {
                if (currentSrc !== fallbackSrc) setCurrentSrc(fallbackSrc);
            }}
        />
    );
}
