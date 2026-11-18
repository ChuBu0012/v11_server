// =============================================================================
// Pocket Sentinel — Three.js 3D Sentinel Energy Core Engine
// =============================================================================

document.addEventListener('alpine:init', () => {
  Alpine.data('threeCoreComponent', () => ({
    initialized: false,
    statusText: 'Core Standby',
    dotStyle: { background: '#2997ff', boxShadow: '0 0 8px #2997ff' },
    threeCore: {
      scene: null,
      camera: null,
      renderer: null,
      controls: null,
      outerMesh: null,
      innerMesh: null,
      ringMesh: null,
      particles: null,
      coreLight: null,
      targetColor: null,
      currentColor: null,
      targetEmissive: null,
      currentEmissive: null,
      baseRotationSpeed: 0.007,
      isPulsing: false,
      pulseTime: 0,
      animId: null,
      isPaused: false
    },

    init() {
      this.initThree();
      window.addEventListener('sentinel:current-loaded', (e) => {
        this.updateCoreState(e.detail);
      });
    },

    initThree() {
      if (this.initialized) return;
      if (typeof THREE === 'undefined') {
        window.addEventListener('load', () => {
          if (typeof THREE !== 'undefined' && !this.initialized) this.initThree();
          else if (typeof THREE === 'undefined') {
            const visual = document.getElementById('hero-visual');
            if (visual) visual.style.display = 'none';
          }
        }, { once: true });
        return;
      }

      const container = document.getElementById('core-canvas-container');
      if (!container) return;

      const width = container.clientWidth || 260;
      const height = container.clientHeight || 220;

      const tc = this.threeCore;
      tc.scene = new THREE.Scene();
      tc.camera = new THREE.PerspectiveCamera(45, width / height, 0.1, 100);
      tc.camera.position.set(0, 0, 5.0);

      tc.renderer = new THREE.WebGLRenderer({ antialias: true, alpha: true, powerPreference: 'low-power' });
      tc.renderer.setSize(width, height);
      tc.renderer.setPixelRatio(Math.min(window.devicePixelRatio || 1, 2));
      tc.renderer.toneMapping = THREE.ACESFilmicToneMapping;
      tc.renderer.toneMappingExposure = 1.15;
      container.appendChild(tc.renderer.domElement);

      tc.targetColor = new THREE.Color(0x2997ff);
      tc.currentColor = new THREE.Color(0x2997ff);
      tc.targetEmissive = new THREE.Color(0x002d62);
      tc.currentEmissive = new THREE.Color(0x002d62);

      const outerGeo = new THREE.IcosahedronGeometry(1.38, 2);
      const outerMat = new THREE.MeshStandardMaterial({
        color: tc.currentColor,
        wireframe: true,
        roughness: 0.25,
        metalness: 0.8,
        transparent: true,
        opacity: 0.88
      });
      tc.outerMesh = new THREE.Mesh(outerGeo, outerMat);
      tc.scene.add(tc.outerMesh);

      const innerGeo = new THREE.SphereGeometry(0.70, 32, 32);
      const innerMat = new THREE.MeshStandardMaterial({
        color: tc.currentColor,
        emissive: tc.currentEmissive,
        emissiveIntensity: 0.95,
        roughness: 0.1,
        metalness: 0.4,
        transparent: true,
        opacity: 0.94
      });
      tc.innerMesh = new THREE.Mesh(innerGeo, innerMat);
      tc.scene.add(tc.innerMesh);

      const ringGeo = new THREE.TorusGeometry(1.82, 0.015, 16, 100);
      const ringMat = new THREE.MeshBasicMaterial({
        color: 0x00aaff,
        transparent: true,
        opacity: 0.45
      });
      tc.ringMesh = new THREE.Mesh(ringGeo, ringMat);
      tc.ringMesh.rotation.x = Math.PI / 3;
      tc.scene.add(tc.ringMesh);

      const particleCount = 120;
      const particleGeo = new THREE.BufferGeometry();
      const positions = new Float32Array(particleCount * 3);
      for (let i = 0; i < particleCount; i++) {
        const angle = Math.random() * Math.PI * 2;
        const radius = 1.45 + Math.random() * 0.85;
        const ySpread = (Math.random() - 0.5) * 0.45;
        positions[i * 3] = Math.cos(angle) * radius;
        positions[i * 3 + 1] = ySpread;
        positions[i * 3 + 2] = Math.sin(angle) * radius;
      }
      particleGeo.setAttribute('position', new THREE.BufferAttribute(positions, 3));
      const particleMat = new THREE.PointsMaterial({
        size: 0.035,
        color: 0x8cd3ff,
        transparent: true,
        opacity: 0.75
      });
      tc.particles = new THREE.Points(particleGeo, particleMat);
      tc.scene.add(tc.particles);

      const ambientLight = new THREE.AmbientLight(0xffffff, 0.7);
      tc.scene.add(ambientLight);

      tc.coreLight = new THREE.PointLight(0x0071e3, 3.2, 10);
      tc.scene.add(tc.coreLight);

      const dirLight = new THREE.DirectionalLight(0xffffff, 1.2);
      dirLight.position.set(3, 4, 5);
      tc.scene.add(dirLight);

      if (typeof THREE.OrbitControls !== 'undefined') {
        tc.controls = new THREE.OrbitControls(tc.camera, tc.renderer.domElement);
        tc.controls.enableDamping = true;
        tc.controls.dampingFactor = 0.05;
        tc.controls.enableZoom = false;
        tc.controls.autoRotate = false;
      }

      const animate = () => {
        tc.animId = requestAnimationFrame(animate);
        if (tc.isPaused) return;

        tc.currentColor.lerp(tc.targetColor, 0.05);
        tc.currentEmissive.lerp(tc.targetEmissive, 0.05);

        if (tc.outerMesh) {
          tc.outerMesh.material.color.copy(tc.currentColor);
          tc.outerMesh.rotation.x += tc.baseRotationSpeed * 0.75;
          tc.outerMesh.rotation.y += tc.baseRotationSpeed;
        }

        if (tc.innerMesh) {
          tc.innerMesh.material.color.copy(tc.currentColor);
          tc.innerMesh.material.emissive.copy(tc.currentEmissive);
          tc.innerMesh.rotation.x -= tc.baseRotationSpeed * 1.2;
          tc.innerMesh.rotation.z += tc.baseRotationSpeed * 0.9;
        }

        if (tc.ringMesh) {
          tc.ringMesh.rotation.z += tc.baseRotationSpeed * 0.45;
        }

        if (tc.particles) {
          tc.particles.rotation.y -= tc.baseRotationSpeed * 0.35;
        }

        if (tc.coreLight) {
          tc.coreLight.color.copy(tc.currentColor);
        }

        if (tc.isPulsing) {
          tc.pulseTime += 0.08;
          const scaleMod = 1.0 + Math.sin(tc.pulseTime) * 0.08;
          if (tc.innerMesh) tc.innerMesh.scale.set(scaleMod, scaleMod, scaleMod);
        } else if (tc.innerMesh) {
          tc.innerMesh.scale.set(1, 1, 1);
        }

        if (tc.controls) tc.controls.update();
        tc.renderer.render(tc.scene, tc.camera);
      };

      this.initialized = true;
      animate();

      const observer = new IntersectionObserver((entries) => {
        entries.forEach(entry => {
          tc.isPaused = !entry.isIntersecting;
        });
      }, { threshold: 0.1 });
      observer.observe(container);

      window.addEventListener('resize', () => {
        if (!tc.renderer || !container) return;
        const w = container.clientWidth || 260;
        const h = container.clientHeight || 220;
        tc.camera.aspect = w / h;
        tc.camera.updateProjectionMatrix();
        tc.renderer.setSize(w, h);
      });
    },

    updateCoreState(data) {
      if (!this.initialized) return;

      const tc = this.threeCore;
      const sample = data ? data.latest_sample : null;
      const conditions = data ? (data.conditions || []) : [];

      let stateName = 'operational';
      if (!sample) stateName = 'standby';
      else if (data.last_error) stateName = 'error';
      else {
        const hasCritical = conditions.some(c => c.active && (c.type === 'Battery Low' || c.type === 'Memory High'));
        const hasWarning = conditions.some(c => c.active);
        if (hasCritical) stateName = 'critical';
        else if (hasWarning) stateName = 'warning';
      }

      if (stateName === 'operational') {
        tc.targetColor.setHex(0x30d158);
        tc.targetEmissive.setHex(0x0a3d12);
        tc.baseRotationSpeed = 0.007;
        tc.isPulsing = false;
        this.dotStyle = { background: '#30d158', boxShadow: '0 0 8px #30d158' };
        this.statusText = 'Core Active · Normal';
      } else if (stateName === 'warning') {
        tc.targetColor.setHex(0xff9f0a);
        tc.targetEmissive.setHex(0x592d00);
        tc.baseRotationSpeed = 0.014;
        tc.isPulsing = true;
        this.dotStyle = { background: '#ff9f0a', boxShadow: '0 0 8px #ff9f0a' };
        this.statusText = 'Core Pulse · Elevated';
      } else if (stateName === 'critical' || stateName === 'error') {
        tc.targetColor.setHex(0xff453a);
        tc.targetEmissive.setHex(0x610c06);
        tc.baseRotationSpeed = 0.026;
        tc.isPulsing = true;
        this.dotStyle = { background: '#ff453a', boxShadow: '0 0 10px #ff453a' };
        this.statusText = 'Core Warning · Action Req';
      } else {
        tc.targetColor.setHex(0x2997ff);
        tc.targetEmissive.setHex(0x002d62);
        tc.baseRotationSpeed = 0.005;
        tc.isPulsing = false;
        this.dotStyle = { background: '#2997ff', boxShadow: '0 0 8px #2997ff' };
        this.statusText = 'Core Standby';
      }
    }
  }));
});
