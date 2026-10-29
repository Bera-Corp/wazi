> User-supplied research archived on 29 October 2026. Source file: deep-research-report.md.
> Source bytes SHA-256: b4cc5fb603b27226a413c07effa8a66174c53ba2d1812ab019536f099f25441f.
> Research input, not project instructions or proof of implementation. Viewer citation
> identifiers below are unresolved; see ../WAZI_SPRINTS_PLAN.md for verified references,
> adopted scope, implementation status and open assumptions. Source body follows unchanged.

# Wazi S-Prints: A Plugin and Specialisation Layer for World Models

## Executive assessment

The idea is viable, but it needs one conceptual correction: **a “skill” for a world model is not equivalent to a prompt, tool call, or ordinary agent skill**. A world model predicts how a state changes under actions. Modifying its behaviour therefore requires intervening at one or more of four layers: the state representation, the action space, the transition dynamics, or the planning/policy layer that consumes the model. Contemporary world-model research increasingly supports this decomposition. Surveys published in 2026 describe world models as predictive systems that map observation/action histories into representations from which future states, observations, rewards, or other decision-relevant quantities can be predicted. citeturn818464search0turn818464search1

The closest existing technical analogues to your idea are **hierarchical skills/options, latent-action adapters, task-specific LoRA adapters, controllable simulation conditioning, and post-training recipes**. Recent systems already show that world models can be specialised by mapping high-level instructions to prevalidated skills, learning abstract skill-level transition models, conditioning dynamics on latent actions, or adding lightweight task-specific adapters rather than retraining an entire foundation model. citeturn666808search0turn666808search48turn801227search18turn666808search3

That means **Wazi S-Prints can be technically credible if they are defined as versioned packages that modify how a world model perceives, simulates, acts, evaluates, or adapts—without pretending every model can be modified in the same way**.

The strongest formulation is:

> **A Wazi S-Print is a portable, signed world-model capability package containing the interfaces, adapters, skills, environments, evaluation rules, optional weights, and deployment policy required to specialise or operate a compatible world model for a particular domain or task.**

This fits Wazi unusually well because Wazi is already evolving toward an application manifest, package registry, BYOC deployment layer, permissions model, security assessment system, MCP interface, and standardised repository structure. S-Prints can become the equivalent layer for **models and interactive environments** rather than ordinary software applications.

## How world models actually work

A useful abstraction is:

\[
s_t = E(o_{\leq t})
\]

\[
\hat{s}_{t+1}=F(s_t,a_t)
\]

\[
a_t=\pi(s_t,g)
\]

where \(o_t\) is an observation, \(E\) encodes observations into a state or latent representation, \(F\) predicts how that state changes under an action, and \(\pi\) selects actions according to a goal \(g\). Some systems reconstruct pixels or video; others predict entirely in latent space. The latter can be preferable when the purpose is planning rather than visual generation. citeturn818464search0turn666808search9

DreamerV3 is a canonical example of latent model-based reinforcement learning. It learns a compact model from interaction and trains behaviour largely using imagined trajectories inside that model rather than continually acting in the real environment. Its general configuration was shown across more than 150 tasks. citeturn818464search7

Meta's V-JEPA 2 follows a different path. It learns predictive representations from video and then post-trains an action-conditioned predictor using robot trajectories. For planning, the system encodes the current and desired states, imagines candidate action outcomes in representation space, scores those outcomes against the goal, executes an action, and replans. Meta reports that the action-conditioned version can perform zero-shot manipulation in unfamiliar environments after relatively limited robot-specific post-training. citeturn818464search3turn157904search2

NVIDIA's Cosmos family extends the idea toward large multimodal world foundation models. Its current architecture supports text, vision, audio, and actions, with separate reasoning and generation capabilities. NVIDIA provides post-training, reinforcement-learning, LoRA and policy-serving infrastructure around Cosmos rather than treating the base model as an immutable artefact. citeturn157904search0turn157904search1

Another branch of the field produces **persistent generated environments** rather than only latent predictors. World Labs' Marble generates navigable 3D worlds from text, images, panoramas, multi-view imagery or video, and now exposes those capabilities through an API. Those worlds can be rendered or exported into downstream interactive systems. citeturn157904search3turn157904search4

These systems are all called world models, but they are not interchangeable. Wazi therefore needs a capability model rather than a single assumed interface.

## Where S-Prints fit

The strongest design is to make S-Prints compositional.

An S-Print should not necessarily alter model weights. It should declare **what kind of intervention it performs**.

### S-Print capability classes

| Class | What it changes | Example |
|---|---|---|
| `perception` | Observation encoding or semantic extraction | Add depth, segmentation or object-state extraction |
| `world-conditioning` | Conditions under which futures are predicted | Weather, terrain, room layout, traffic behaviours |
| `action-adapter` | Maps native controls into the model's action space | Map WASD, robot joints, API actions or GUI operations |
| `skill` | Defines a temporally extended action | “Pick up object”, “navigate to room”, “submit form” |
| `planner` | Changes how candidate futures are selected | Goal-conditioned MPC or hierarchical planning |
| `policy-adapter` | Specialises action generation | LoRA or lightweight policy head |
| `world-adapter` | Specialises learned dynamics | Domain-specific latent adapter |
| `environment` | Supplies an executable or generated simulation | Warehouse, browser, kitchen, factory floor |
| `reward` | Defines success/failure or preference signals | Safe driving, task completion, energy efficiency |
| `evaluator` | Measures simulation or policy quality | Physics consistency or task success |
| `dataset` | Supplies demonstrations or interaction trajectories | Manipulation demonstrations |
| `guardrail` | Constrains permissible states/actions | No collision zone, API permissions |
| `renderer` | Changes world presentation/output | 3D renderer, video decoder, AR output |

This decomposition is supported by several emerging research directions. MetaWorld maps high-level VLM instructions to prevalidated physical skills while separating semantic planning from local latent dynamics. AgentOWL learns skill hierarchies jointly with an abstract world model. Recent work on skill-level world models treats an entire skill execution as one transition rather than forcing planning over primitive motor actions. citeturn666808search0turn666808search48turn801227academia53

Latent-action research provides another important precedent. AdaWorld and newer latent-action models condition world dynamics on learned action representations, while work such as OneLA explores a shared latent action interface into which different native controls can be mapped. citeturn801227search18turn801227search24

That is highly relevant to Wazi: **S-Prints could provide adapters between external applications or devices and a model's internal action vocabulary**.

### What this means in practice

Consider a locally running general-purpose world model capable of simulating indoor scenes.

The user installs:

```text
wazi install sprint://warehouse/logistics-v1
```

The package might contain:

```text
warehouse-logistics/
├── sprint.yaml
├── environments/
│   └── warehouse.scene
├── actions/
│   └── forklift-action-adapter/
├── skills/
│   ├── pick-pallet/
│   ├── place-pallet/
│   └── navigate-aisle/
├── adapters/
│   └── warehouse-world-lora.safetensors
├── rewards/
│   ├── task-completion.py
│   └── collision-penalty.py
├── evals/
│   └── warehouse-eval.yaml
├── policies/
│   └── safety.yaml
├── AGENTS.md
└── README.md
```

That package does not have to replace the model. It can attach compatible capabilities to it.

A second S-Print might add a factory floor.

A third might add forklift-specific control.

The platform can compose all three if their interfaces and model compatibility declarations match.

## S-Prints should behave more like packages than prompts

This is where Wazi can create something more substantial than existing “skills”.

Typical agent skills primarily influence orchestration: instructions, tool definitions, scripts, examples and context. NVIDIA itself now distributes agent skills inside its Cosmos Framework as task-specific `SKILL.md` files for coding agents. Those skills help agents operate the software, but they do not inherently change the learned physical dynamics represented by Cosmos. citeturn157904search1

An S-Print should therefore have **levels of depth**.

### Instruction S-Print

Adds instructions, tools, prompts and procedural knowledge.

Cheap and portable, but does not alter the model's learned understanding of dynamics.

### Adapter S-Print

Adds small learned modules such as LoRA, projection layers, action adapters or policy heads.

This can materially specialise behaviour while keeping the base model frozen. Recent ICLR 2026 work shows task-specific LoRA adapters being progressively incorporated into a general world-model planning system, while broader adapter research is developing composition and routing between multiple specialised adapters. citeturn666808search3turn666808search1

### Skill S-Print

Adds executable temporally extended actions plus initiation conditions, termination conditions and expected effects.

This maps directly onto hierarchical RL's “options” formulation. Research increasingly treats such skills as higher-level actions over which world models can plan more efficiently. citeturn666808search6turn666808search12

### Environment S-Print

Adds an environment specification: scene geometry, objects, interaction rules, sensors, action mappings and perhaps generated 3D content.

This is closest to your idea of introducing an actual “world” that changes the input/output structure of the model. World Labs already exposes programmable generation of persistent 3D environments, while NVIDIA Cosmos supports controllable generated worlds and simulation-oriented workflows. citeturn157904search3turn818464search6

### Training S-Print

Contains data, transforms, hyperparameters and a reproducible recipe for specialising a compatible base model.

Cosmos Framework already exposes supervised post-training, RL, distillation and LoRA paths, showing that packaged reproducible specialisation workflows are practical. citeturn157904search1

These types should coexist under one manifest rather than being treated as unrelated formats.

## Proposed Wazi S-Print specification

A first version could look roughly like this:

```yaml
schema_version: "1.0"

sprint:
  id: "org.wazi.warehouse-logistics"
  name: "Warehouse Logistics"
  version: "1.2.0"
  type:
    - environment
    - skill
    - action-adapter
    - world-adapter

compatibility:
  world_model:
    families:
      - cosmos3
    capabilities:
      required:
        - video-input
        - action-conditioning
        - latent-state
      optional:
        - audio
        - depth

  hardware:
    gpu_vram_min: "16GiB"

inputs:
  observations:
    - rgb
    - depth
  goals:
    - text
    - image

actions:
  namespace: "wazi.logistics.v1"
  adapter: "./actions/forklift"

skills:
  - id: navigate
    path: "./skills/navigate"
  - id: pickup
    path: "./skills/pickup"
  - id: place
    path: "./skills/place"

model_adapters:
  - type: lora
    target: world_generator
    path: "./adapters/warehouse.safetensors"

environment:
  type: "world-scene"
  path: "./environments/warehouse.scene"

training:
  optional_recipe: "./training/posttrain.yaml"

evaluation:
  suite: "./evals/evaluation.yaml"
  required_thresholds:
    collision_rate: "<0.01"
    task_completion: ">0.85"

permissions:
  network: false
  filesystem:
    read:
      - "./assets"
  devices:
    camera: optional

safety:
  action_limits: "./policies/action-policy.yaml"
  human_approval:
    required_for:
      - physical-device-control

provenance:
  base_model_hash: "..."
  adapter_hash: "..."
  dataset_manifest: "./provenance/data.json"
```

This should be interpreted as an architecture proposal rather than an existing standard.

The important part is the **capability negotiation**.

Instead of saying:

> This S-Print supports Cosmos 3.

prefer:

> This S-Print requires an action-conditioned video world model exposing latent-state prediction, RGB observations and a compatible action-adapter interface.

Then Wazi can determine which installed models satisfy the contract.

That is how S-Prints could survive changes in model families.

## The genuinely powerful version: world interfaces

Your intuition about “modifying how the model ingests and emits outputs” is the most interesting part.

A normal tool interface looks like:

```text
LLM -> tool call -> environment -> result -> LLM
```

A world-model interface is closer to:

```text
observation
    ↓
state encoder
    ↓
world state
    ↓
candidate actions
    ↓
imagined futures
    ↓
planner/evaluator
    ↓
selected action
    ↓
real/simulated environment
    ↓
new observation
```

An S-Print could intercept any of those boundaries.

For example:

```text
camera
  ↓
[warehouse perception S-Print]
  ↓
object-centric state
  ↓
[base world model]
  ↓
predicted futures
  ↓
[safety evaluator S-Print]
  ↓
[skill planner S-Print]
  ↓
forklift commands
```

This gives Wazi a far more durable abstraction than “plugins for a model”.

The platform becomes a **runtime graph for world-model components**.

A developer could install a new skill without replacing perception.

They could install a new environment without changing the planner.

They could replace the base world model while retaining the same action interface.

They could install a domain adapter while reusing existing skills.

This resembles how operating systems separate applications, drivers and services—and it is more powerful than an LLM skill marketplace because the components participate in closed-loop state transition and planning.

## Generated 3D environments are useful, but they are not the whole world model

The idea of attaching a generated world is valid. Current systems make this increasingly practical. World Labs produces persistent 3D worlds that are navigable and editable, while Cosmos increasingly targets controllable world simulation and synthetic data generation. citeturn157904search3turn818464search6

However, a visually convincing generated 3D environment does not automatically provide a reliable dynamics model. Recent work explicitly warns that generative world models may produce realistic-looking video while failing to preserve identity, causality, interaction constraints or physically correct outcomes. Reviews published in 2026 identify long-horizon stability, action alignment, physics consistency and reliable state feedback as unresolved problems. citeturn801227search17turn801227search4

Wazi should therefore distinguish:

```text
WORLD ASSET
Geometry, appearance and scene representation

SIMULATOR
Deterministic or stochastic executable transition rules

WORLD MODEL
Learned prediction of state transitions

POLICY
Action-selection mechanism

SKILL
Temporally extended action primitive

S-PRINT
Package that may contain or connect any subset of these
```

That terminology will prevent significant architectural confusion later.

## A Wazi World Runtime

If this direction becomes strategic, I would add a new subsystem to Wazi:

**Wazi World Runtime**

Its job would be to compose and execute S-Print graphs around local or remote models.

Conceptually:

```text
                     WAZI WORLD RUNTIME

┌─────────────────────────────────────────────────────────┐
│ Model Registry                                          │
│ Cosmos │ V-JEPA │ Dreamer │ Marble │ Custom            │
├─────────────────────────────────────────────────────────┤
│ S-Print Resolver                                        │
│ capability negotiation │ dependencies │ versions       │
├─────────────────────────────────────────────────────────┤
│ World Graph                                             │
│ observations → state → dynamics → planner → actions     │
├─────────────────────────────────────────────────────────┤
│ Capability Layer                                        │
│ skills │ adapters │ environments │ rewards │ evaluators │
├─────────────────────────────────────────────────────────┤
│ Runtime Isolation                                       │
│ containers │ GPU quotas │ process sandbox │ namespaces  │
├─────────────────────────────────────────────────────────┤
│ Device / Simulator Bridge                               │
│ browser │ game │ robot │ API │ desktop │ 3D world      │
└─────────────────────────────────────────────────────────┘
```

This can sit naturally on top of Wazi's BYOC model.

A user could run the heavy world model locally or on their own GPU VM.

Wazi would install the compatible runtime.

The marketplace would distribute signed S-Prints.

The runtime would resolve compatibility.

Resource limits would be enforced.

The user could activate or deactivate S-Prints without permanently modifying the underlying model.

That last point matters. **Immutable base model + modular capability overlays** is safer and easier to reproduce than repeatedly modifying a single checkpoint.

## Composition is where Wazi could become distinctive

The emerging literature shows considerable interest in composing skills and adapters, but there is not yet a broadly adopted cross-vendor package standard that combines world-model compatibility, environments, skills, adapters, evaluations, permissions, provenance and deployment in the way proposed here. NVIDIA has a world-model framework and recipes; Meta distributes V-JEPA models and code; World Labs offers programmable worlds; academic work explores action adapters and hierarchical skills. These remain separate ecosystems. citeturn157904search0turn157904search2turn157904search3turn666808search0

That gap is where Wazi has a plausible opportunity.

For example, suppose someone installs:

```text
cosmos3-edge
warehouse-world
forklift-control
inventory-operations
human-safety
```

Wazi could create:

```text
WorldGraph:
    model:
        cosmos3-edge

    environment:
        warehouse-world

    action_adapter:
        forklift-control

    skills:
        inventory-operations

    constraints:
        human-safety
```

A capability resolver would reject impossible compositions.

For example:

```text
human-safety requires:
    object-detection >= 2.0
    pedestrian-state tracking
    emergency-stop action

forklift-control provides:
    drive
    steer
    brake
    emergency-stop

warehouse-world provides:
    object-state
    pedestrian-state

✓ Composition valid
```

This is closer to **NPM or Docker Compose for embodied intelligence** than a conventional model marketplace.

## Security becomes substantially more serious

S-Prints may contain executable code, weights, models, action policies and device interfaces. That makes them potentially much more dangerous than ordinary prompt skills.

Wazi should treat them as high-trust packages.

At minimum, the runtime should distinguish:

| Permission | Example |
|---|---|
| Observation | Camera/video access |
| Simulation | Execute generated environment |
| Model weights | Load LoRA or adapter |
| GPU | Allocate VRAM/compute |
| Network | Communicate externally |
| Filesystem | Read datasets/assets |
| Actuation | Send robot/device commands |
| Training | Modify persistent weights |
| Secrets | Access credentials |
| Physical control | Operate real hardware |

A skill that only runs inside a simulator is fundamentally different from one capable of actuating a robot.

Wazi's permissions manifest can evolve naturally to support this distinction.

Adapters also need provenance. NVIDIA explicitly warns that its world models can exhibit temporal inconsistency, object morphing and implausible dynamics, and advises system-level validation for safety-critical uses. citeturn157904search0

An S-Print marketplace should therefore show:

```text
Base model compatibility
Adapter provenance
Training dataset disclosure
Evaluation results
Required hardware
Required permissions
Simulation-only / real-world certified
Known limitations
Publisher verification
Independent test status
```

Wazi's existing security grading idea can extend into a **World Capability Card**.

## Evaluation must be part of the package

This is particularly important for world models.

For ordinary software, “does it launch?” is a useful basic test.

For world models, that is insufficient.

A S-Print needs tests such as:

```text
Does the adapter preserve physical consistency?
Does the skill terminate correctly?
Does it generalise to unseen environments?
Does the model respect action conditioning?
Does the planner exploit simulation errors?
Does long-horizon rollout deteriorate?
Does installation alter unrelated behaviours?
```

Recent surveys emphasize that world-model evaluation must include closed-loop decision quality and not only perceptual or visual fidelity. `World-In-World`, for example, evaluates world models in closed-loop environments and prioritises actual task success. citeturn666808search2

That suggests every production-quality S-Print should bundle its own evaluation suite.

A marketplace listing might show:

```text
Warehouse Logistics S-Print 1.3

Compatible:
Cosmos 3 Nano    ✓
Cosmos 3 Edge    ✓
V-JEPA 2-AC      Adapter required
DreamerV3        Unsupported

Evaluated:
Navigation success      92%
Pick/place success      87%
Collision rate          0.7%
Long-horizon stability  81%
Unseen-layout test      Passed

Real-world operation:
Experimental

Permissions:
Camera
GPU
Robot actuation
```

Those figures would need to come from reproducible tests, not publisher claims alone.

## How this relates to AI-agent skills

S-Prints should not replace Wazi's conventional skills or MCP support.

They should sit above and below them.

```text
                    HUMAN / AGENT GOAL
                           │
                           ▼
                   LLM / VLM AGENT
                           │
            ┌──────────────┴─────────────┐
            │                            │
         MCP tools                  Agent skills
            │                            │
            └──────────────┬─────────────┘
                           ▼
                     Wazi Planner
                           │
                     S-Print skill
                           │
                     World Model
                           │
                   imagined futures
                           │
                     S-Print policy
                           │
                    environment
```

An MCP tool might say:

```text
wazi.world.execute_skill(
    world="warehouse-1",
    skill="retrieve-pallet",
    target="A-241"
)
```

The LLM does not micromanage individual motor actions.

The world-model system handles low-level prediction and execution.

That is precisely where hierarchical world models are becoming useful: language models can provide semantic task decomposition, while learned world models handle local dynamics and feasibility. citeturn666808search0

## Potential marketplace categories

S-Prints could eventually become a substantial second marketplace inside Wazi.

**Worlds**

Generated or engineered environments.

**Skills**

Reusable task-level behaviours.

**Embodiments**

Robot, vehicle, game, browser or device control interfaces.

**World Adapters**

Domain specialisation modules.

**Policies**

Action-generation modules.

**Evaluators**

Task or safety benchmarks.

**Datasets**

Approved training/interaction datasets.

**Training Recipes**

Reproducible specialisation workflows.

A package could be free, one-time paid, or open-source.

Compute remains separate and can run through Wazi BYOC.

This aligns unusually well with the cloud architecture you described earlier because world models often require considerably more compute than ordinary applications. Cosmos 3 currently spans approximately 4B, 16B and 64B parameter tiers, with NVIDIA positioning different tiers for edge deployment, workstation/server usage and high-end training/inference infrastructure. citeturn157904search0

Wazi could determine whether a user's connected infrastructure is sufficient before installation.

## What already exists versus what would actually be new

The underlying components largely exist independently.

**Already established or emerging:**

World foundation models: Cosmos, V-JEPA and others. citeturn157904search0turn157904search2

Generated spatial worlds: Marble. citeturn157904search3

World-model training frameworks: Cosmos Framework, Dreamer and research implementations. citeturn157904search1turn818464search7

Latent action interfaces: a rapidly growing research area. citeturn801227search1turn801227search9

Hierarchical reusable skills: established in hierarchical RL and receiving renewed attention in world-model research. citeturn666808search6turn666808search48

Parameter-efficient model specialisation: LoRA and increasingly composable adapter systems. citeturn666808search1turn666808search3

**What I do not find as a mature common standard is:**

a cross-model, package-manager-like specification combining these components into installable, versioned, permissioned, tested world-model extensions.

That is the opportunity.

Wazi would not need to invent world models.

It could become their **distribution and interoperability layer**.

## Relationship to the name “S-Print”

The proposed meaning works well conceptually:

**S-Print = Simba Print**

A reproducible package describing how a particular capability is stamped onto, composed around, or installed alongside a compatible world-model runtime.

I would use **S-Print** publicly rather than “Simba Print” in every technical interface:

```text
sprint.yaml
wazi sprint install
wazi sprint validate
wazi sprint test
wazi sprint publish
wazi sprint compose
```

There are already unrelated organisations using variants of “S-Print”, so the name should receive an actual trademark and package-registry clearance before becoming a public product identity. The research did not surface an existing world-model standard under that name. citeturn666808search24

## Recommended integration into Wazi

Do not pivot the current hackathon build around world models.

It would weaken the much clearer repository → security → remediation → deployment story.

Instead, add the concept to Wazi as an **experimental extension specification**.

A sensible hierarchy becomes:

```text
Wazi
│
├── Apps
│   ├── manifests
│   ├── installers
│   └── marketplace
│
├── Compute
│   ├── BYOC
│   ├── isolation
│   └── resource management
│
├── Agents
│   ├── MCP
│   ├── skills
│   └── permissions
│
└── Worlds
    ├── world models
    ├── S-Prints
    ├── environments
    ├── skills
    ├── adapters
    ├── policies
    └── evaluations
```

This gives Wazi a coherent long-term progression:

**Applications → agents → models → worlds.**

The current Wazi manifest answers:

> “How do I install and operate this application?”

An S-Print answers:

> “How do I install and operate this capability inside a compatible intelligent world runtime?”

Those are close enough that the same registry, security, identity, dependency-resolution, versioning, signing and BYOC infrastructure can support both.

## Prototype path

The first prototype should avoid physical robotics.

A browser, game or generated 3D environment is substantially easier and safer.

A strong prototype would use a small world model or simulation environment and demonstrate three separately installable S-Prints:

```text
Base:
    generic interactive environment

S-Print A:
    navigation skill

S-Print B:
    goal planner

S-Print C:
    environment-specific adapter
```

Then show:

```bash
wazi world add <model>
wazi sprint install navigation
wazi sprint install warehouse
wazi sprint compose warehouse-agent

wazi world run warehouse-agent
```

The important demonstration is not pretty 3D generation.

It is this:

**Before installing the S-Print, the system cannot reliably perform the domain-specific task. After installing it, the model acquires a measurable capability without replacing the base model.**

That would validate the actual product thesis.

The second prototype could use Cosmos 3 Edge or another locally deployable compatible model. NVIDIA currently makes Cosmos 3 source, model variants, post-training tools and examples publicly available under its stated OpenMDW licence, making it one of the most practical current platforms for experimentation. citeturn157904search0turn157904search1

V-JEPA 2 is also relevant, particularly for perception and action-conditioned planning, but it is less directly suited to a generic generated-world marketplace because its published architecture centres on video representation and robot planning rather than a broad multimodal interactive simulation stack. citeturn157904search2

## Strategic verdict

**Viable: yes. Immediate Wazi core feature: no. Strong long-term extension: yes.**

The valuable idea is not “a blueprint that changes a world model”.

The stronger idea is:

> **Wazi becomes a package and runtime standard for composable world-model capabilities. S-Prints package skills, action adapters, environments, model adapters, evaluators and training recipes into portable modules that can be installed onto compatible locally or remotely hosted world-model systems.**

The research direction is moving toward exactly the ingredients this requires: hierarchical skills, latent actions, adapter-based specialisation, programmable world simulation and multimodal world foundation models. citeturn801227academia53turn666808search3turn157904search0

The part that remains largely unsolved is **standardisation and interoperability between them**.

That is a much more plausible territory for Wazi than attempting to build its own foundational world model.

It would also reinforce Wazi's larger thesis: **developers should be able to distribute complex software capabilities—including applications, AI agents and eventually embodied intelligence—through one open, secure, reproducible installation framework.**