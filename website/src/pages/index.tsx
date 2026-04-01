import type {ReactNode} from 'react';
import {useEffect, useState} from 'react';
import clsx from 'clsx';
import Link from '@docusaurus/Link';
import useDocusaurusContext from '@docusaurus/useDocusaurusContext';
import Layout from '@theme/Layout';
import HomepageFeatures from '@site/src/components/HomepageFeatures';
import Heading from '@theme/Heading';
import CodeBlock from '@theme/CodeBlock';

import styles from './index.module.css';

const GITHUB_REPO = 'kubeopencode/kubeopencode';

const agentYaml = `apiVersion: kubeopencode.io/v1alpha1
kind: Agent
metadata:
  name: dev-agent
spec:
  profile: "Interactive development agent"
  workspaceDir: /workspace
  port: 4096
  persistence:
    sessions:
      size: "2Gi"
  idleTimeout: "30m"`;

const taskYaml = `apiVersion: kubeopencode.io/v1alpha1
kind: Task
metadata:
  name: update-dependencies
spec:
  templateRef:
    name: ci-runner
  description: |
    Update all dependencies to latest versions.
    Run tests and create a pull request.`;

function useGitHubStars(): number | null {
  const [stars, setStars] = useState<number | null>(null);
  useEffect(() => {
    fetch(`https://api.github.com/repos/${GITHUB_REPO}`)
      .then(res => res.json())
      .then(data => {
        if (typeof data.stargazers_count === 'number') {
          setStars(data.stargazers_count);
        }
      })
      .catch(() => {});
  }, []);
  return stars;
}

function AlphaBanner() {
  return (
    <div className={styles.alphaBanner}>
      <div className="container">
        This project is in <strong>early alpha</strong> (v0.0.x). Not recommended for production use. API may change without backward compatibility.
      </div>
    </div>
  );
}

function StarIcon({size = 16}: {size?: number}) {
  return (
    <svg width={size} height={size} viewBox="0 0 16 16" fill="currentColor" style={{marginRight: '0.4rem', verticalAlign: 'text-bottom'}}>
      <path d="M8 .25a.75.75 0 01.673.418l1.882 3.815 4.21.612a.75.75 0 01.416 1.279l-3.046 2.97.719 4.192a.75.75 0 01-1.088.791L8 12.347l-3.766 1.98a.75.75 0 01-1.088-.79l.72-4.194L.818 6.374a.75.75 0 01.416-1.28l4.21-.611L7.327.668A.75.75 0 018 .25z"/>
    </svg>
  );
}

function HomepageHeader() {
  const {siteConfig} = useDocusaurusContext();
  const stars = useGitHubStars();
  return (
    <header className={clsx('hero hero--primary', styles.heroBanner)}>
      <div className="container">
        <Heading as="h1" className="hero__title">
          {siteConfig.title}
        </Heading>
        <p className="hero__subtitle">{siteConfig.tagline}</p>
        <p className={styles.heroDescription}>
          Run AI agents on Kubernetes.
          Built on OpenCode, designed for teams and enterprise.
        </p>
        <div className={styles.buttons}>
          <Link
            className="button button--secondary button--lg"
            to="/docs/getting-started">
            Get Started
          </Link>
          <Link
            className={styles.starBadge}
            href={`https://github.com/${GITHUB_REPO}`}>
            <span className={styles.starBadgeLeft}>
              <StarIcon size={16} />
              Star
            </span>
            {stars !== null && (
              <span className={styles.starBadgeCount}>{stars}</span>
            )}
          </Link>
        </div>
      </div>
    </header>
  );
}

function QuickExample() {
  return (
    <section className={styles.quickExample}>
      <div className="container">
        <div className="row">
          <div className="col col--6">
            <Heading as="h2">Live Agent: Human-in-the-Loop</Heading>
            <p>
              Deploy persistent AI agents your team can interact with in real time
              &mdash; through the web terminal, CLI, or by submitting Tasks.
            </p>
            <ul>
              <li>Zero cold start &mdash; agent is always running</li>
              <li>Interactive terminal access via CLI or web</li>
              <li>Auto-suspend when idle, resume on demand</li>
              <li>Session history persists across restarts</li>
            </ul>
          </div>
          <div className="col col--6">
            <CodeBlock language="yaml" title="agent.yaml">
              {agentYaml}
            </CodeBlock>
          </div>
        </div>
        <div className="row" style={{marginTop: '2rem'}}>
          <div className="col col--6">
            <Heading as="h2">AgentTemplate: Workflows at Scale</Heading>
            <p>
              Run stable, repeatable AI tasks in ephemeral Pods. Perfect for
              CI/CD pipelines, batch operations, and automated workflows.
            </p>
            <ul>
              <li>No new tools to learn &mdash; just <code>kubectl apply</code></li>
              <li>Works with any CI/CD pipeline</li>
              <li>Scale with Helm templates for batch operations</li>
              <li>Rate limiting and quota controls</li>
            </ul>
          </div>
          <div className="col col--6">
            <CodeBlock language="yaml" title="task.yaml">
              {taskYaml}
            </CodeBlock>
          </div>
        </div>
      </div>
    </section>
  );
}

export default function Home(): ReactNode {
  return (
    <Layout
      title="Kubernetes-native Agent Platform for Teams and Enterprise"
      description="Run AI agents on Kubernetes. Built on OpenCode, designed for teams and enterprise.">
      <AlphaBanner />
      <HomepageHeader />
      <main>
        <HomepageFeatures />
        <QuickExample />
      </main>
    </Layout>
  );
}
